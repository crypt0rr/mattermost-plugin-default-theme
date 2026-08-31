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
	testAPI.AssertExpectations(t)
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
	testAPI.AssertExpectations(t)
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
