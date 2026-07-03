package remoteserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigFileMissingReturnsDefaults(t *testing.T) {
	cfg, err := loadConfigFile(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("loadConfigFile() err=%v", err)
	}
	if cfg != DefaultConfig() {
		t.Fatalf("expected default config, got %+v", cfg)
	}
}

func TestLoadOrInitConfigFileMissingCreatesToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "remote_server.json")

	cfg, err := loadOrInitConfigFile(path)
	if err != nil {
		t.Fatalf("loadOrInitConfigFile() err=%v", err)
	}
	if !cfg.Enabled {
		t.Fatalf("expected enabled config")
	}
	if cfg.Token == "" {
		t.Fatalf("expected generated token")
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() err=%v", err)
	}
	if !strings.Contains(string(saved), cfg.Token) {
		t.Fatalf("expected token persisted to config file")
	}
}

func TestLoadConfigFileNormalizesValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "remote_server.json")
	if err := os.WriteFile(path, []byte(`{"enabled":true,"bindAddress":"","port":0,"token":"  abc  ","triggerCooldownSec":0}`), 0o644); err != nil {
		t.Fatalf("WriteFile() err=%v", err)
	}

	cfg, err := loadConfigFile(path)
	if err != nil {
		t.Fatalf("loadConfigFile() err=%v", err)
	}
	if !cfg.Enabled {
		t.Fatalf("expected enabled config")
	}
	if cfg.BindAddress != defaultBindAddress {
		t.Fatalf("expected bind address %q, got %q", defaultBindAddress, cfg.BindAddress)
	}
	if cfg.Port != defaultPort {
		t.Fatalf("expected port %d, got %d", defaultPort, cfg.Port)
	}
	if cfg.Token != "abc" {
		t.Fatalf("expected trimmed token, got %q", cfg.Token)
	}
	if cfg.TriggerCooldownSec != defaultTriggerCooldownSec {
		t.Fatalf("expected cooldown %d, got %d", defaultTriggerCooldownSec, cfg.TriggerCooldownSec)
	}
}

func TestLoadOrInitConfigFileAddsTokenWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "remote_server.json")
	if err := os.WriteFile(path, []byte(`{"enabled":true,"bindAddress":"0.0.0.0","port":18680}`), 0o644); err != nil {
		t.Fatalf("WriteFile() err=%v", err)
	}

	cfg, err := loadOrInitConfigFile(path)
	if err != nil {
		t.Fatalf("loadOrInitConfigFile() err=%v", err)
	}
	if cfg.Token == "" {
		t.Fatalf("expected generated token")
	}
}

func TestConfigValidateRequiresTokenWhenEnabled(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected token validation error")
	}
	cfg.Token = "secret"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}
