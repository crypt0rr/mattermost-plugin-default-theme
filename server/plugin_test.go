package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const testTheme = `{"sidebarBg":"#145DBF","sidebarText":"#FFFFFF"}`

func TestUserHasBeenCreatedAppliesConfiguredTheme(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	p.setConfiguration(&configuration{DefaultTheme: testTheme})

	updatePreferences := testAPI.On("UpdatePreferencesForUser", "user-id", mock.AnythingOfType("[]model.Preference"))
	updatePreferences.Return(nil).Once()
	updatePreferences.Run(func(args mock.Arguments) {
		require.Equal(t, "user-id", args.String(0))
		require.Equal(t, []model.Preference{{
			UserId:   "user-id",
			Category: model.PreferenceCategoryTheme,
			Name:     "",
			Value:    testTheme,
		}}, args.Get(1))
	})

	p.UserHasBeenCreated(nil, &model.User{Id: "user-id"})
	testAPI.AssertExpectations(t)
}

func TestUserHasBeenCreatedSkipsBots(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	p.setConfiguration(&configuration{DefaultTheme: testTheme})

	p.UserHasBeenCreated(nil, &model.User{Id: "bot-id", IsBot: true})
	testAPI.AssertExpectations(t)
}

func TestUserHasBeenCreatedSkipsWhenThemeIsBlank(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)

	p.UserHasBeenCreated(nil, &model.User{Id: "user-id"})
	testAPI.AssertExpectations(t)
}

func TestUserHasBeenCreatedLogsPreferenceError(t *testing.T) {
	testAPI := &plugintest.API{}
	p := Plugin{}
	p.SetAPI(testAPI)
	p.setConfiguration(&configuration{DefaultTheme: testTheme})

	testAPI.On("UpdatePreferencesForUser", "user-id", mock.AnythingOfType("[]model.Preference")).Return(&model.AppError{Message: "preference unavailable"}).Once()
	testAPI.On("LogError", `failed to apply default theme to user user-id: preference unavailable`).Once()

	p.UserHasBeenCreated(nil, &model.User{Id: "user-id"})
	testAPI.AssertExpectations(t)
}

func TestUserHasBeenCreatedHandlesNilUser(t *testing.T) {
	p := Plugin{}
	p.UserHasBeenCreated(nil, nil)
	require.Empty(t, p.getConfiguration().DefaultTheme)
}
