package main

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestConfigurationClone(t *testing.T) {
	t.Run("nil configuration", func(t *testing.T) {
		var original *configuration

		clone := original.Clone()

		require.NotNil(t, clone)
		require.Empty(t, clone.DefaultTheme)
	})

	t.Run("copies configuration", func(t *testing.T) {
		theme := `{"sidebarBg":"#145DBF"}`
		original := &configuration{DefaultTheme: theme}

		clone := original.Clone()
		original.DefaultTheme = "changed"

		require.Equal(t, theme, clone.DefaultTheme)
		require.NotSame(t, original, clone)
	})
}

func TestSetConfigurationHandlesNil(t *testing.T) {
	p := Plugin{}

	p.setConfiguration(nil)

	require.Empty(t, p.getConfiguration().DefaultTheme)
}

func TestGetConfigurationReturnsSnapshot(t *testing.T) {
	p := Plugin{}
	p.setConfiguration(&configuration{DefaultTheme: `{"sidebarBg":"#145DBF"}`})

	snapshot := p.getConfiguration()
	snapshot.DefaultTheme = "changed"

	require.Equal(t, `{"sidebarBg":"#145DBF"}`, p.getConfiguration().DefaultTheme)
}

func configWithPluginSettings(settings map[string]any) *model.Config {
	return &model.Config{
		PluginSettings: model.PluginSettings{
			Plugins: map[string]map[string]any{
				defaultThemePluginID: settings,
			},
		},
	}
}

func TestValidateTargetTheme(t *testing.T) {
	tests := []struct {
		name     string
		username string
		theme    string
		wantErr  string
	}{
		{name: "blank request"},
		{name: "missing username", theme: testTheme, wantErr: "target username is required"},
		{name: "missing theme", username: "alice", wantErr: "target theme is required"},
		{name: "valid request", username: "alice", theme: testTheme},
		{name: "invalid theme", username: "alice", theme: `{"sidebarBg":123}`, wantErr: "must be a string"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTargetTheme(test.username, test.theme)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestConfigurationFromPluginSettings(t *testing.T) {
	t.Run("decodes settings", func(t *testing.T) {
		settings := map[string]any{
			"DefaultTheme":   testTheme,
			"TargetUsername": "alice",
			"TargetTheme":    `{"sidebarBg":"#FFFFFF"}`,
		}

		configuration, err := configurationFromPluginSettings(settings)

		require.NoError(t, err)
		require.Equal(t, testTheme, configuration.DefaultTheme)
		require.Equal(t, "alice", configuration.TargetUsername)
		require.Equal(t, `{"sidebarBg":"#FFFFFF"}`, configuration.TargetTheme)
	})

	t.Run("rejects values that cannot be encoded", func(t *testing.T) {
		_, err := configurationFromPluginSettings(map[string]any{"DefaultTheme": func() {}})

		require.ErrorContains(t, err, "failed to encode plugin configuration")
	})

	t.Run("rejects values that cannot be decoded", func(t *testing.T) {
		_, err := configurationFromPluginSettings(map[string]any{"DefaultTheme": 123})

		require.ErrorContains(t, err, "failed to decode plugin configuration")
	})
}

func TestConfigWithTargetThemeRequestClearedPreservesSettings(t *testing.T) {
	original := configWithPluginSettings(map[string]any{
		"DefaultTheme":   testTheme,
		"TargetUsername": "alice",
		"TargetTheme":    `{"sidebarBg":"#FFFFFF"}`,
		"OtherSetting":   "preserved",
	})
	original.PluginSettings.Plugins["other.plugin"] = map[string]any{"value": "preserved"}

	cleared := configWithTargetThemeRequestCleared(original)

	require.NotSame(t, original, cleared)
	require.Empty(t, cleared.PluginSettings.Plugins[defaultThemePluginID]["TargetUsername"])
	require.Empty(t, cleared.PluginSettings.Plugins[defaultThemePluginID]["TargetTheme"])
	require.Equal(t, testTheme, cleared.PluginSettings.Plugins[defaultThemePluginID]["DefaultTheme"])
	require.Equal(t, "preserved", cleared.PluginSettings.Plugins[defaultThemePluginID]["OtherSetting"])
	require.Equal(t, map[string]any{"value": "preserved"}, cleared.PluginSettings.Plugins["other.plugin"])
	require.Equal(t, "alice", original.PluginSettings.Plugins[defaultThemePluginID]["TargetUsername"])
	require.Equal(t, `{"sidebarBg":"#FFFFFF"}`, original.PluginSettings.Plugins[defaultThemePluginID]["TargetTheme"])
}

func TestConfigWithTargetThemeRequestClearedHandlesMattermostSettingKeyCasing(t *testing.T) {
	original := configWithPluginSettings(map[string]any{
		"defaulttheme":   testTheme,
		"targetusername": "alice",
		"targettheme":    `{"sidebarBg":"#FFFFFF"}`,
		"OtherSetting":   "preserved",
	})

	cleared := configWithTargetThemeRequestCleared(original)
	settings := cleared.PluginSettings.Plugins[defaultThemePluginID]

	require.Empty(t, settings["targetusername"])
	require.Empty(t, settings["targettheme"])
	require.Equal(t, testTheme, settings["defaulttheme"])
	require.Equal(t, "preserved", settings["OtherSetting"])
	_, hasUppercaseUsernameKey := settings["TargetUsername"]
	_, hasUppercaseThemeKey := settings["TargetTheme"]
	require.False(t, hasUppercaseUsernameKey)
	require.False(t, hasUppercaseThemeKey)
	require.Equal(t, "alice", original.PluginSettings.Plugins[defaultThemePluginID]["targetusername"])
	require.Equal(t, `{"sidebarBg":"#FFFFFF"}`, original.PluginSettings.Plugins[defaultThemePluginID]["targettheme"])
}

func TestConfigWithTargetThemeRequestClearedAddsMissingRequestKeys(t *testing.T) {
	original := configWithPluginSettings(map[string]any{
		"DefaultTheme": testTheme,
		"OtherSetting": "preserved",
	})

	cleared := configWithTargetThemeRequestCleared(original)
	settings := cleared.PluginSettings.Plugins[defaultThemePluginID]

	require.Empty(t, settings["TargetUsername"])
	require.Empty(t, settings["TargetTheme"])
	require.Equal(t, testTheme, settings["DefaultTheme"])
	require.Equal(t, "preserved", settings["OtherSetting"])
}

func TestValidateTheme(t *testing.T) {
	tests := []struct {
		name    string
		theme   string
		wantErr string
	}{
		{name: "blank", theme: ""},
		{name: "object", theme: `{"sidebarBg":"#145DBF","type":"custom"}`},
		{name: "empty object", theme: `{}`},
		{name: "malformed", theme: `{"sidebarBg"`, wantErr: "valid JSON"},
		{name: "null", theme: `null`, wantErr: "JSON object"},
		{name: "array", theme: `[]`, wantErr: "valid JSON"},
		{name: "empty string value", theme: `{"sidebarBg":""}`},
		{name: "number value", theme: `{"sidebarBg":123}`, wantErr: "must be a string"},
		{name: "boolean value", theme: `{"sidebarBg":true}`, wantErr: "must be a string"},
		{name: "array value", theme: `{"sidebarBg":[]}`, wantErr: "must be a string"},
		{name: "object value", theme: `{"sidebarBg":{}}`, wantErr: "must be a string"},
		{name: "null value", theme: `{"sidebarBg":null}`, wantErr: "must be a string"},
		{name: "trailing value", theme: `{} {}`, wantErr: "valid JSON"},
		{name: "too long", theme: `{"key":"` + strings.Repeat("a", model.MaxPreferenceValueLength) + `"}`, wantErr: "exceeds"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTheme(test.theme)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.Contains(t, err.Error(), test.wantErr)
		})
	}
}

func TestValidateThemeRespectsRuneLengthLimit(t *testing.T) {
	const prefix = `{"key":"`
	const suffix = `"}`
	theme := prefix + strings.Repeat("界", model.MaxPreferenceValueLength-len(prefix)-len(suffix)) + suffix

	require.NoError(t, validateTheme(theme))
	require.ErrorContains(t, validateTheme(theme+"界"), "exceeds")
}

func TestOnConfigurationChangeLoadsValidTheme(t *testing.T) {
	theme := `{"sidebarBg":"#145DBF"}`
	configuredTheme := "\n\t" + theme + "  \n"
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)

	loadConfiguration := testAPI.On("LoadPluginConfiguration", mock.AnythingOfType("*main.configuration"))
	loadConfiguration.Return(nil).Run(func(args mock.Arguments) {
		args.Get(0).(*configuration).DefaultTheme = configuredTheme
	})

	require.NoError(t, p.OnConfigurationChange())
	require.Equal(t, theme, p.getConfiguration().DefaultTheme)
}

func TestOnConfigurationChangeClearsPreviousTheme(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	p.setConfiguration(&configuration{DefaultTheme: `{"sidebarBg":"#145DBF"}`})

	loadConfiguration := testAPI.On("LoadPluginConfiguration", mock.AnythingOfType("*main.configuration"))
	loadConfiguration.Return(nil).Run(func(args mock.Arguments) {
		args.Get(0).(*configuration).DefaultTheme = " \n\t"
	})

	require.NoError(t, p.OnConfigurationChange())
	require.Empty(t, p.getConfiguration().DefaultTheme)
}

func TestOnConfigurationChangeKeepsLastValidThemeOnInvalidTheme(t *testing.T) {
	previousTheme := `{"sidebarBg":"#145DBF"}`
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	p.setConfiguration(&configuration{DefaultTheme: previousTheme})

	loadConfiguration := testAPI.On("LoadPluginConfiguration", mock.AnythingOfType("*main.configuration"))
	loadConfiguration.Return(nil).Run(func(args mock.Arguments) {
		args.Get(0).(*configuration).DefaultTheme = `{"sidebarBg":123}`
	})
	testAPI.On("LogError", mock.MatchedBy(func(message string) bool {
		return message == `invalid default theme configuration: theme value "sidebarBg" must be a string`
	})).Once()

	err := p.OnConfigurationChange()
	require.Error(t, err)
	require.Equal(t, previousTheme, p.getConfiguration().DefaultTheme)
	testAPI.AssertExpectations(t)
}

func TestOnConfigurationChangeKeepsEmptyThemeOnLoadError(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	testAPI.On("LoadPluginConfiguration", mock.AnythingOfType("*main.configuration")).Return(errors.New("configuration unavailable"))

	err := p.OnConfigurationChange()
	require.EqualError(t, err, "failed to load plugin configuration: configuration unavailable")
	require.Empty(t, p.getConfiguration().DefaultTheme)
	testAPI.AssertExpectations(t)
}

func TestOnConfigurationChangeKeepsPreviousThemeOnLoadError(t *testing.T) {
	previousTheme := `{"sidebarBg":"#145DBF"}`
	loadError := errors.New("configuration unavailable")
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	p.setConfiguration(&configuration{DefaultTheme: previousTheme})
	testAPI.On("LoadPluginConfiguration", mock.AnythingOfType("*main.configuration")).Return(loadError).Run(func(args mock.Arguments) {
		args.Get(0).(*configuration).DefaultTheme = `{"sidebarBg":"#FFFFFF"}`
	})

	err := p.OnConfigurationChange()

	require.ErrorIs(t, err, loadError)
	require.Equal(t, previousTheme, p.getConfiguration().DefaultTheme)
	testAPI.AssertExpectations(t)
}

func TestOnConfigurationChangeLoadsTargetFieldsWithoutApplyingThem(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)

	loadConfiguration := testAPI.On("LoadPluginConfiguration", mock.AnythingOfType("*main.configuration"))
	loadConfiguration.Return(nil).Run(func(args mock.Arguments) {
		loaded := args.Get(0).(*configuration)
		loaded.DefaultTheme = testTheme
		loaded.TargetUsername = "alice"
		loaded.TargetTheme = `{"sidebarBg":"#FFFFFF"}`
	})

	require.NoError(t, p.OnConfigurationChange())
	require.Equal(t, configuration{
		DefaultTheme:   testTheme,
		TargetUsername: "alice",
		TargetTheme:    `{"sidebarBg":"#FFFFFF"}`,
	}, p.getConfiguration())
	testAPI.AssertExpectations(t)
}

func TestOnConfigurationChangeResetsAppliedTargetRequestState(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	p.lastAppliedTargetTheme = "already-applied"

	loadConfiguration := testAPI.On("LoadPluginConfiguration", mock.AnythingOfType("*main.configuration"))
	loadConfiguration.Return(nil)

	require.NoError(t, p.OnConfigurationChange())
	require.Empty(t, p.lastAppliedTargetTheme)
	testAPI.AssertExpectations(t)
}

func TestConfigurationWillBeSavedHandlesNilAndMissingPluginConfiguration(t *testing.T) {
	p := Plugin{}

	cleared, err := p.ConfigurationWillBeSaved(nil)
	require.ErrorContains(t, err, "configuration cannot be nil")
	require.Nil(t, cleared)

	empty := &model.Config{}
	unchanged, err := p.ConfigurationWillBeSaved(empty)
	require.NoError(t, err)
	require.Same(t, empty, unchanged)
}

func TestConfigurationWillBeSavedSkipsBlankTargetRequest(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	config := configWithPluginSettings(map[string]any{
		"DefaultTheme":   testTheme,
		"TargetUsername": " \n\t",
		"TargetTheme":    " \t",
	})

	unchanged, err := p.ConfigurationWillBeSaved(config)

	require.NoError(t, err)
	require.Same(t, config, unchanged)
	testAPI.AssertExpectations(t)
}

func TestConfigurationWillBeSavedAppliesThemeAndClearsRequest(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	config := configWithPluginSettings(map[string]any{
		"DefaultTheme":   testTheme,
		"TargetUsername": "  alice  ",
		"TargetTheme":    "\n{\"sidebarBg\":\"#FFFFFF\"}\t",
		"OtherSetting":   "preserved",
	})
	user := &model.User{Id: "user-id", Username: "alice"}

	testAPI.On("GetUserByUsername", "alice").Return(user, (*model.AppError)(nil)).Once()
	testAPI.On("UpdatePreferencesForUser", "user-id", mock.AnythingOfType("[]model.Preference")).Return((*model.AppError)(nil)).Once().Run(func(args mock.Arguments) {
		require.Equal(t, "user-id", args.String(0))
		require.Equal(t, []model.Preference{{
			UserId:   "user-id",
			Category: model.PreferenceCategoryTheme,
			Name:     "",
			Value:    `{"sidebarBg":"#FFFFFF"}`,
		}}, args.Get(1))
	})

	cleared, err := p.ConfigurationWillBeSaved(config)

	require.NoError(t, err)
	require.NotSame(t, config, cleared)
	require.Equal(t, testTheme, cleared.PluginSettings.Plugins[defaultThemePluginID]["DefaultTheme"])
	require.Empty(t, cleared.PluginSettings.Plugins[defaultThemePluginID]["TargetUsername"])
	require.Empty(t, cleared.PluginSettings.Plugins[defaultThemePluginID]["TargetTheme"])
	require.Equal(t, "preserved", cleared.PluginSettings.Plugins[defaultThemePluginID]["OtherSetting"])
	require.Equal(t, "  alice  ", config.PluginSettings.Plugins[defaultThemePluginID]["TargetUsername"])
	require.NotEmpty(t, p.lastAppliedTargetTheme)
	testAPI.AssertExpectations(t)
}

func TestConfigurationWillBeSavedAppliesThemeWithMattermostSettingKeyCasing(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	config := configWithPluginSettings(map[string]any{
		"defaulttheme":   testTheme,
		"targetusername": "alice",
		"targettheme":    testTheme,
	})

	testAPI.On("GetUserByUsername", "alice").Return(&model.User{Id: "user-id", Username: "alice"}, (*model.AppError)(nil)).Once()
	testAPI.On("UpdatePreferencesForUser", "user-id", mock.AnythingOfType("[]model.Preference")).Return((*model.AppError)(nil)).Once().Run(func(args mock.Arguments) {
		require.Equal(t, []model.Preference{{
			UserId:   "user-id",
			Category: model.PreferenceCategoryTheme,
			Name:     "",
			Value:    testTheme,
		}}, args.Get(1))
	})

	cleared, err := p.ConfigurationWillBeSaved(config)

	require.NoError(t, err)
	require.Empty(t, cleared.PluginSettings.Plugins[defaultThemePluginID]["targetusername"])
	require.Empty(t, cleared.PluginSettings.Plugins[defaultThemePluginID]["targettheme"])
	require.Equal(t, testTheme, cleared.PluginSettings.Plugins[defaultThemePluginID]["defaulttheme"])
	testAPI.AssertExpectations(t)
}

func TestConfigurationWillBeSavedAcceptsGuest(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	config := configWithPluginSettings(map[string]any{
		"TargetUsername": "guest",
		"TargetTheme":    testTheme,
	})

	testAPI.On("GetUserByUsername", "guest").Return(&model.User{Id: "guest-id", Username: "guest", Roles: model.SystemGuestRoleId}, (*model.AppError)(nil)).Once()
	testAPI.On("UpdatePreferencesForUser", "guest-id", mock.AnythingOfType("[]model.Preference")).Return((*model.AppError)(nil)).Once()

	cleared, err := p.ConfigurationWillBeSaved(config)

	require.NoError(t, err)
	require.Empty(t, cleared.PluginSettings.Plugins[defaultThemePluginID]["TargetUsername"])
	require.Empty(t, cleared.PluginSettings.Plugins[defaultThemePluginID]["TargetTheme"])
	testAPI.AssertExpectations(t)
}

func TestConfigurationWillBeSavedDoesNotRepeatSameRequest(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	config := configWithPluginSettings(map[string]any{
		"TargetUsername": "alice",
		"TargetTheme":    testTheme,
	})
	p.lastAppliedTargetTheme = targetThemeRequestKey(&configuration{TargetUsername: "alice", TargetTheme: testTheme})

	cleared, err := p.ConfigurationWillBeSaved(config)

	require.NoError(t, err)
	require.Empty(t, cleared.PluginSettings.Plugins[defaultThemePluginID]["TargetUsername"])
	require.Empty(t, cleared.PluginSettings.Plugins[defaultThemePluginID]["TargetTheme"])
	testAPI.AssertExpectations(t)
}

func TestConfigurationWillBeSavedRejectsInvalidPluginConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
		wantErr  string
	}{
		{
			name:     "unencodable settings",
			settings: map[string]any{"DefaultTheme": func() {}},
			wantErr:  "failed to encode plugin configuration",
		},
		{
			name:     "wrong field type",
			settings: map[string]any{"DefaultTheme": 123},
			wantErr:  "failed to decode plugin configuration",
		},
		{
			name:     "incomplete target request",
			settings: map[string]any{"TargetUsername": "alice"},
			wantErr:  "target theme is required",
		},
		{
			name:     "invalid target theme",
			settings: map[string]any{"TargetUsername": "alice", "TargetTheme": `{"sidebarBg":123}`},
			wantErr:  "must be a string",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testAPI := &plugintest.API{}
			p := Plugin{}
			p.SetAPI(testAPI)
			testAPI.On("LogError", mock.MatchedBy(func(message string) bool {
				return strings.Contains(message, test.wantErr)
			})).Once()

			cleared, err := p.ConfigurationWillBeSaved(configWithPluginSettings(test.settings))

			require.ErrorContains(t, err, test.wantErr)
			require.Nil(t, cleared)
			testAPI.AssertExpectations(t)
		})
	}
}

func TestConfigurationWillBeSavedRejectsUnavailableAPI(t *testing.T) {
	p := Plugin{}
	config := configWithPluginSettings(map[string]any{
		"TargetUsername": "alice",
		"TargetTheme":    testTheme,
	})

	cleared, err := p.ConfigurationWillBeSaved(config)

	require.ErrorContains(t, err, "plugin API is unavailable")
	require.Nil(t, cleared)
}

func TestConfigurationWillBeSavedRejectsTargetLookupFailures(t *testing.T) {
	tests := []struct {
		name    string
		user    *model.User
		appErr  *model.AppError
		wantErr string
	}{
		{name: "not found", wantErr: "was not found"},
		{name: "lookup error", appErr: &model.AppError{Message: "lookup unavailable"}, wantErr: "failed to resolve target user"},
		{name: "bot", user: &model.User{Id: "bot-id", IsBot: true}, wantErr: "is a bot"},
		{name: "empty ID", user: &model.User{Username: "alice"}, wantErr: "has no user ID"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testAPI := &plugintest.API{}
			p := Plugin{}
			p.SetAPI(testAPI)
			testAPI.On("GetUserByUsername", "alice").Return(test.user, test.appErr).Once()
			testAPI.On("LogError", mock.MatchedBy(func(message string) bool {
				return strings.Contains(message, test.wantErr)
			})).Once()

			cleared, err := p.ConfigurationWillBeSaved(configWithPluginSettings(map[string]any{
				"TargetUsername": "alice",
				"TargetTheme":    testTheme,
			}))

			require.ErrorContains(t, err, test.wantErr)
			require.Nil(t, cleared)
			testAPI.AssertExpectations(t)
		})
	}
}

func TestConfigurationWillBeSavedRejectsPreferenceFailures(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	preferenceErr := &model.AppError{Message: "preference unavailable"}
	config := configWithPluginSettings(map[string]any{
		"TargetUsername": "alice",
		"TargetTheme":    testTheme,
	})

	testAPI.On("GetUserByUsername", "alice").Return(&model.User{Id: "user-id"}, (*model.AppError)(nil)).Once()
	testAPI.On("UpdatePreferencesForUser", "user-id", mock.AnythingOfType("[]model.Preference")).Return(preferenceErr).Once()
	testAPI.On("LogError", "failed to apply target theme to user user-id: preference unavailable").Once()

	cleared, err := p.ConfigurationWillBeSaved(config)

	require.ErrorContains(t, err, "failed to apply target theme to user user-id")
	require.Nil(t, cleared)
	require.Empty(t, p.lastAppliedTargetTheme)
	require.Equal(t, testTheme, config.PluginSettings.Plugins[defaultThemePluginID]["TargetTheme"])
	testAPI.AssertExpectations(t)
}

func TestConfigurationWillBeSavedSerializesDuplicateRequests(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	config := configWithPluginSettings(map[string]any{
		"TargetUsername": "alice",
		"TargetTheme":    testTheme,
	})

	testAPI.On("GetUserByUsername", "alice").Return(&model.User{Id: "user-id"}, (*model.AppError)(nil)).Once()
	testAPI.On("UpdatePreferencesForUser", "user-id", mock.AnythingOfType("[]model.Preference")).Return((*model.AppError)(nil)).Once()

	results := make([]*model.Config, 2)
	errors := make([]error, 2)
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(results))
	for index := range results {
		go func(index int) {
			defer waitGroup.Done()
			results[index], errors[index] = p.ConfigurationWillBeSaved(config)
		}(index)
	}
	waitGroup.Wait()

	for index := range results {
		require.NoError(t, errors[index])
		require.Empty(t, results[index].PluginSettings.Plugins[defaultThemePluginID]["TargetUsername"])
		require.Empty(t, results[index].PluginSettings.Plugins[defaultThemePluginID]["TargetTheme"])
	}
	testAPI.AssertExpectations(t)
}

func TestConfigurationIsSafeForConcurrentAccess(t *testing.T) {
	p := Plugin{}
	const iterations = 1000
	const firstTheme = `{"sidebarBg":"#145DBF"}`
	const secondTheme = `{"sidebarBg":"#FFFFFF"}`
	p.setConfiguration(&configuration{DefaultTheme: firstTheme})

	var waitGroup sync.WaitGroup
	var invalidRead atomic.Bool
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		for i := 0; i < iterations; i++ {
			theme := firstTheme
			if i%2 == 1 {
				theme = secondTheme
			}
			p.setConfiguration(&configuration{DefaultTheme: theme})
		}
	}()
	go func() {
		defer waitGroup.Done()
		for i := 0; i < iterations; i++ {
			theme := p.getConfiguration().DefaultTheme
			if theme != firstTheme && theme != secondTheme {
				invalidRead.Store(true)
			}
		}
	}()

	waitGroup.Wait()
	require.False(t, invalidRead.Load())
}
