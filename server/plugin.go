package main

import (
	"fmt"
	"sync"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// Plugin implements the hooks used by Mattermost to apply the configured theme.
type Plugin struct {
	plugin.MattermostPlugin

	configurationLock sync.RWMutex
	configuration     *configuration
}

// UserHasBeenCreated applies the configured theme to a newly created human user.
func (p *Plugin) UserHasBeenCreated(_ *plugin.Context, user *model.User) {
	if user == nil || user.IsBot {
		return
	}

	config := p.getConfiguration()
	if config.DefaultTheme == "" {
		return
	}

	preferences := []model.Preference{
		{
			UserId:   user.Id,
			Category: model.PreferenceCategoryTheme,
			Name:     "",
			Value:    config.DefaultTheme,
		},
	}

	if appErr := p.API.UpdatePreferencesForUser(user.Id, preferences); appErr != nil {
		p.API.LogError(fmt.Sprintf("failed to apply default theme to user %s: %s", user.Id, appErr.Error()))
	}
}
