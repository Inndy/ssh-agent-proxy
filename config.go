package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CachePolicy string

const (
	CacheNone    CachePolicy = "none"
	CacheDura    CachePolicy = "duration"
	CacheForever CachePolicy = "forever"
)

type UpstreamConfig struct {
	Name          string      `json:"name"`
	Socket        string      `json:"socket"`
	Cache         CachePolicy `json:"cache"`
	CacheDuration string      `json:"cache_duration"`

	cacheDuration time.Duration
}

type LogConfig struct {
	Enabled bool   `json:"enabled"`
	File    string `json:"file"`
	Level   string `json:"level"`
}

type Config struct {
	Listen    string           `json:"listen"`
	Upstreams []UpstreamConfig `json:"upstreams"`
	Log       LogConfig        `json:"log"`
}

func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

func defaultConfigPath() string {
	return expandTilde("~/.config/ssh-agent-proxy/config.json")
}

func defaultConfig() *Config {
	return &Config{
		Listen: "/tmp/ssh-agent-proxy.sock",
		Upstreams: []UpstreamConfig{
			{
				Name:   "system",
				Socket: os.Getenv("SSH_AUTH_SOCK"),
				Cache:  CacheNone,
			},
		},
		Log: LogConfig{
			Enabled: true,
			File:    "/tmp/ssh-agent-proxy.log",
			Level:   "info",
		},
	}
}

func createDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	cfg := defaultConfig()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling default config: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("writing default config: %w", err)
	}
	return nil
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config: %w", err)
		}
		if createErr := createDefaultConfig(path); createErr != nil {
			return nil, createErr
		}
		fmt.Fprintf(os.Stderr, "Default config created at %s\nPlease edit it and restart.\n", path)
		os.Exit(0)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Listen == "" {
		return nil, fmt.Errorf("listen path is required")
	}
	cfg.Listen = expandTilde(cfg.Listen)

	if len(cfg.Upstreams) == 0 {
		return nil, fmt.Errorf("at least one upstream is required")
	}

	for i := range cfg.Upstreams {
		u := &cfg.Upstreams[i]
		if u.Name == "" {
			u.Name = fmt.Sprintf("upstream-%d", i)
		}
		u.Socket = expandTilde(u.Socket)

		switch u.Cache {
		case CacheNone, "":
			u.Cache = CacheNone
		case CacheDura:
			if u.CacheDuration == "" {
				return nil, fmt.Errorf("upstream %q: cache_duration required for duration cache", u.Name)
			}
			d, err := time.ParseDuration(u.CacheDuration)
			if err != nil {
				return nil, fmt.Errorf("upstream %q: invalid cache_duration: %w", u.Name, err)
			}
			u.cacheDuration = d
		case CacheForever:
		default:
			return nil, fmt.Errorf("upstream %q: unknown cache policy %q", u.Name, u.Cache)
		}
	}

	if cfg.Log.File != "" {
		cfg.Log.File = expandTilde(cfg.Log.File)
	}

	return &cfg, nil
}
