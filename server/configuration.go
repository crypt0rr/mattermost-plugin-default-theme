package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mattermost/mattermost/server/public/model"
)

// configuration contains the settings exposed through the Mattermost System Console.
type configuration struct {
	DefaultTheme string
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
	return nil
}

func validateConfiguration(c *configuration) error {
	if err := validateTheme(c.DefaultTheme); err != nil {
		return fmt.Errorf("invalid default theme configuration: %w", err)
	}

	return nil
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
