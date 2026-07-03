package remoteserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"pause/internal/paths"
)

const (
	defaultBindAddress        = "0.0.0.0"
	defaultPort               = 18680
	defaultTriggerCooldownSec = 60
	configFileName            = "remote_server.json"
)

// Config defines the local/LAN HTTP control surface settings.
type Config struct {
	Enabled            bool   `json:"enabled"`
	BindAddress        string `json:"bindAddress"`
	Port               int    `json:"port"`
	Token              string `json:"token,omitempty"`
	TriggerCooldownSec int    `json:"triggerCooldownSec,omitempty"`
}

// DefaultConfig returns the safe default remote server configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:            true,
		BindAddress:        defaultBindAddress,
		Port:               defaultPort,
		TriggerCooldownSec: defaultTriggerCooldownSec,
	}
}

// LoadConfig loads remote server settings from the app config directory.
func LoadConfig() (Config, error) {
	path, err := paths.ConfigFile(configFileName)
	if err != nil {
		return Config{}, err
	}
	return loadConfigFile(path)
}

func loadConfigFile(path string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg.Normalize(), nil
}

// Normalize clamps invalid config values to safe defaults.
func (c Config) Normalize() Config {
	if strings.TrimSpace(c.BindAddress) == "" {
		c.BindAddress = defaultBindAddress
	}
	if c.Port <= 0 || c.Port > 65535 {
		c.Port = defaultPort
	}
	c.Token = strings.TrimSpace(c.Token)
	if c.TriggerCooldownSec <= 0 {
		c.TriggerCooldownSec = defaultTriggerCooldownSec
	}
	return c
}

// Validate rejects insecure or inconsistent configurations.
func (c Config) Validate() error {
	c = c.Normalize()
	if !c.Enabled {
		return nil
	}
	if c.Token == "" {
		return errors.New("remote server token is required when enabled")
	}
	return nil
}

// LocalBaseURL returns the loopback URL for same-machine access.
func (c Config) LocalBaseURL() string {
	c = c.Normalize()
	return fmt.Sprintf("http://127.0.0.1:%d", c.Port)
}
