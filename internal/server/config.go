package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config holds server configuration.
type Config struct {
	Port          int    `yaml:"port"`
	Bind          string `yaml:"bind"`
	DBUrl         string `yaml:"db_url"`
	TLSCert       string `yaml:"tls_cert"`
	TLSKey        string `yaml:"tls_key"`
	AdminPassword string `yaml:"admin_password"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Port: 8377,
		Bind: "0.0.0.0",
	}
}

// LoadConfig loads server config from ~/.ctx/server.yaml, falling back to defaults.
// Environment variables override file values: CTX_SERVER_PORT, CTX_SERVER_BIND,
// CTX_SERVER_DB_URL, CTX_SERVER_TLS_CERT, CTX_SERVER_TLS_KEY.
func LoadConfig() Config {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err == nil {
		path := filepath.Join(home, ".ctx", "server.yaml")
		data, err := os.ReadFile(path)
		if err == nil {
			_ = yaml.Unmarshal(data, &cfg)
		}
	}

	// Environment variables override file config
	if v := os.Getenv("CTX_SERVER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Port = n
		}
	}
	if v := os.Getenv("CTX_SERVER_BIND"); v != "" {
		cfg.Bind = v
	}
	if v := os.Getenv("CTX_SERVER_DB_URL"); v != "" {
		cfg.DBUrl = v
	}
	if v := os.Getenv("CTX_SERVER_TLS_CERT"); v != "" {
		cfg.TLSCert = v
	}
	if v := os.Getenv("CTX_SERVER_TLS_KEY"); v != "" {
		cfg.TLSKey = v
	}
	if v := os.Getenv("CTX_SERVER_ADMIN_PASSWORD"); v != "" {
		cfg.AdminPassword = v
	}

	return cfg
}

// Addr returns the listen address as "bind:port".
func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Bind, c.Port)
}

// HasTLS returns true if both TLS cert and key are configured.
func (c Config) HasTLS() bool {
	return c.TLSCert != "" && c.TLSKey != ""
}
