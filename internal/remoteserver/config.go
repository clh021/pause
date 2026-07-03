package remoteserver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}
	return loadOrInitConfigFile(path)
}

func ConfigPath() (string, error) {
	return paths.ConfigFile(configFileName)
}

func loadConfigFile(path string) (Config, error) {
	cfg, _, err := readConfigFile(path)
	return cfg, err
}

func loadOrInitConfigFile(path string) (Config, error) {
	cfg, _, err := readConfigFile(path)
	if err != nil {
		return Config{}, err
	}
	if !cfg.Enabled || cfg.Token != "" {
		return cfg, nil
	}

	token, err := generateToken()
	if err != nil {
		return Config{}, err
	}
	cfg.Token = token
	if err := writeConfigFile(path, cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func readConfigFile(path string) (Config, bool, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, false, nil
		}
		return Config{}, false, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return cfg, true, nil
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, true, err
	}
	return cfg.Normalize(), true, nil
}

func writeConfigFile(path string, cfg Config) error {
	cfg = cfg.Normalize()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func generateToken() (string, error) {
	var buf [24]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
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
