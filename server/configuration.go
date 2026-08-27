package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mattermost/mattermost/server/public/model"
)

const defaultThemePluginID = "com.github.crypt0rr.default-theme"

// configuration contains the settings exposed through the Mattermost System Console.
type configuration struct {
	DefaultTheme   string
	TargetUsername string
	TargetTheme    string
}

func (c *configuration) Clone() *configuration {
	if c == nil {
		return &configuration{}
	}

	clone := *c
	return &clone
}

func (p *Plugin) getConfiguration() configuration {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()

	if p.configuration == nil {
		return configuration{}
	}

	return *p.configuration
}

func (p *Plugin) setConfiguration(next *configuration) {
	p.configurationLock.Lock()
	defer p.configurationLock.Unlock()

	p.configuration = next.Clone()
}

func (c *configuration) normalize() {
	c.DefaultTheme = strings.TrimSpace(c.DefaultTheme)
	c.TargetUsername = strings.TrimSpace(c.TargetUsername)
	c.TargetTheme = strings.TrimSpace(c.TargetTheme)
}

func (c configuration) hasTargetThemeRequest() bool {
	return c.TargetUsername != "" || c.TargetTheme != ""
}

func (p *Plugin) resetTargetThemeRequestState() {
	p.targetThemeRequestLock.Lock()
	defer p.targetThemeRequestLock.Unlock()

	p.lastAppliedTargetTheme = ""
}

// OnConfigurationChange loads and validates the System Console settings.
func (p *Plugin) OnConfigurationChange() error {
	next := new(configuration)
	if err := p.API.LoadPluginConfiguration(next); err != nil {
		return fmt.Errorf("failed to load plugin configuration: %w", err)
	}
	next.normalize()

	if err := validateConfiguration(next); err != nil {
		p.API.LogError(err.Error())
		return err
	}

	p.setConfiguration(next)
	if !next.hasTargetThemeRequest() {
		p.resetTargetThemeRequestState()
	}
	return nil
}

func validateConfiguration(c *configuration) error {
	if err := validateTheme(c.DefaultTheme); err != nil {
		return fmt.Errorf("invalid default theme configuration: %w", err)
	}

	if err := validateTargetTheme(c.TargetUsername, c.TargetTheme); err != nil {
		return fmt.Errorf("invalid target theme configuration: %w", err)
	}

	return nil
}

func validateTargetTheme(username, theme string) error {
	if username == "" && theme == "" {
		return nil
	}
	if username == "" {
		return errors.New("target username is required when target theme is provided")
	}
	if theme == "" {
		return errors.New("target theme is required when target username is provided")
	}

	return validateTheme(theme)
}

func configurationFromPluginSettings(settings map[string]any) (*configuration, error) {
	encoded, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to encode plugin configuration: %w", err)
	}

	result := new(configuration)
	if err := json.Unmarshal(encoded, result); err != nil {
		return nil, fmt.Errorf("failed to decode plugin configuration: %w", err)
	}

	result.normalize()
	return result, nil
}

func targetThemeRequestKey(c *configuration) string {
	return c.TargetUsername + "\x00" + c.TargetTheme
}

func configWithTargetThemeRequestCleared(original *model.Config) *model.Config {
	clone := *original
	clone.PluginSettings = original.PluginSettings
	clone.PluginSettings.Plugins = make(map[string]map[string]any, len(original.PluginSettings.Plugins))
	for pluginID, settings := range original.PluginSettings.Plugins {
		clone.PluginSettings.Plugins[pluginID] = settings
	}

	settings := make(map[string]any, len(original.PluginSettings.Plugins[defaultThemePluginID])+2)
	usernameKeyFound := false
	themeKeyFound := false
	for key, value := range original.PluginSettings.Plugins[defaultThemePluginID] {
		if strings.EqualFold(key, "TargetUsername") {
			settings[key] = ""
			usernameKeyFound = true
			continue
		}
		if strings.EqualFold(key, "TargetTheme") {
			settings[key] = ""
			themeKeyFound = true
			continue
		}
		settings[key] = value
	}
	if !usernameKeyFound {
		settings["TargetUsername"] = ""
	}
	if !themeKeyFound {
		settings["TargetTheme"] = ""
	}
	clone.PluginSettings.Plugins[defaultThemePluginID] = settings

	return &clone
}

// ConfigurationWillBeSaved applies a one-shot per-user theme request before the
// proposed configuration is persisted, and clears the request from the saved config.
func (p *Plugin) ConfigurationWillBeSaved(newCfg *model.Config) (*model.Config, error) {
	if newCfg == nil {
		return nil, errors.New("configuration cannot be nil")
	}

	pluginSettings, ok := newCfg.PluginSettings.Plugins[defaultThemePluginID]
	if !ok {
		return newCfg, nil
	}

	next, err := configurationFromPluginSettings(pluginSettings)
	if err != nil {
		p.API.LogError(fmt.Sprintf("invalid plugin configuration: %s", err.Error()))
		return nil, err
	}
	if err := validateConfiguration(next); err != nil {
		p.API.LogError(err.Error())
		return nil, err
	}
	if !next.hasTargetThemeRequest() {
		return newCfg, nil
	}
	if p.API == nil {
		return nil, errors.New("plugin API is unavailable")
	}

	p.targetThemeRequestLock.Lock()
	defer p.targetThemeRequestLock.Unlock()

	requestKey := targetThemeRequestKey(next)
	if requestKey == p.lastAppliedTargetTheme {
		return configWithTargetThemeRequestCleared(newCfg), nil
	}

	user, appErr := p.API.GetUserByUsername(next.TargetUsername)
	if appErr != nil {
		err := fmt.Errorf("failed to resolve target user %q: %w", next.TargetUsername, appErr)
		p.API.LogError(fmt.Sprintf("failed to resolve target user %q: %s", next.TargetUsername, appErr.Error()))
		return nil, err
	}
	if user == nil {
		err := fmt.Errorf("target user %q was not found", next.TargetUsername)
		p.API.LogError(err.Error())
		return nil, err
	}
	if user.IsBot {
		err := fmt.Errorf("target user %q is a bot and cannot receive an administrator theme", next.TargetUsername)
		p.API.LogError(fmt.Sprintf("target user %q (%s) is a bot and cannot receive an administrator theme", next.TargetUsername, user.Id))
		return nil, err
	}
	if user.Id == "" {
		err := fmt.Errorf("target user %q has no user ID", next.TargetUsername)
		p.API.LogError(err.Error())
		return nil, err
	}

	preferences := []model.Preference{
		{
			UserId:   user.Id,
			Category: model.PreferenceCategoryTheme,
			Name:     "",
			Value:    next.TargetTheme,
		},
	}
	if appErr := p.API.UpdatePreferencesForUser(user.Id, preferences); appErr != nil {
		err := fmt.Errorf("failed to apply target theme to user %s: %w", user.Id, appErr)
		p.API.LogError(fmt.Sprintf("failed to apply target theme to user %s: %s", user.Id, appErr.Error()))
		return nil, err
	}

	p.lastAppliedTargetTheme = requestKey
	return configWithTargetThemeRequestCleared(newCfg), nil
}

func validateTheme(theme string) error {
	if theme == "" {
		return nil
	}

	if utf8.RuneCountInString(theme) > model.MaxPreferenceValueLength {
		return fmt.Errorf("theme JSON exceeds %d characters", model.MaxPreferenceValueLength)
	}

	var values map[string]json.RawMessage
	if err := json.Unmarshal([]byte(theme), &values); err != nil {
		return fmt.Errorf("theme must be valid JSON: %w", err)
	}
	if values == nil {
		return fmt.Errorf("theme must be a JSON object")
	}

	for name, rawValue := range values {
		var value *string
		if err := json.Unmarshal(rawValue, &value); err != nil || value == nil {
			return fmt.Errorf("theme value %q must be a string", name)
		}
	}

	return nil
}
