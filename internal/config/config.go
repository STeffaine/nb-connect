package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	NetBox   NetBoxConfig   `yaml:"netbox"`
	Services ServicesConfig `yaml:"services"`
	SSH      SSHConfig      `yaml:"ssh"`
	Cache    CacheConfig    `yaml:"cache"`
	Ping     PingConfig     `yaml:"ping"`
}

type NetBoxConfig struct {
	Servers []NetBoxServer `yaml:"servers"`
}

type NetBoxServer struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type ServicesConfig struct {
	Enabled []string `yaml:"enabled"`
}

type SSHConfig struct {
	DefaultUser string                  `yaml:"default_user"`
	Keys        map[string]SSHKeyConfig `yaml:"keys"`
}

type SSHKeyConfig struct {
	IdentityFile string `yaml:"identity_file"`
}

type CacheConfig struct {
	TTL time.Duration `yaml:"-"`
}

type PingConfig struct {
	Count int `yaml:"count"`
}

type rawConfig struct {
	NetBox   NetBoxConfig   `yaml:"netbox"`
	Services ServicesConfig `yaml:"services"`
	SSH      SSHConfig      `yaml:"ssh"`
	Ping     PingConfig     `yaml:"ping"`
	Cache    struct {
		TTL string `yaml:"ttl"`
	} `yaml:"cache"`
}

type Credentials struct {
	NetBox struct {
		Servers map[string]NetBoxServerCredentials `yaml:"servers"`
	} `yaml:"netbox"`
}

type NetBoxServerCredentials struct {
	Token string `yaml:"token"`
}

func DefaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(dir, "nb-connect", "config.yaml"), nil
}

func DefaultCredentialsPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "credentials.yaml")
}

func Load(path string) (Config, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read configuration %q: %w", path, err)
	}

	var raw rawConfig
	if err := yaml.Unmarshal(contents, &raw); err != nil {
		return Config{}, fmt.Errorf("parse configuration %q: %w", path, err)
	}

	servers := raw.NetBox.Servers
	if len(servers) == 0 {
		return Config{}, errors.New("netbox.servers is required")
	}
	serverNames := make(map[string]struct{}, len(servers))
	for index := range servers {
		server := &servers[index]
		server.Name = strings.TrimSpace(server.Name)
		server.URL = strings.TrimRight(strings.TrimSpace(server.URL), "/")
		if server.Name == "" {
			return Config{}, fmt.Errorf("netbox.servers[%d].name is required", index)
		}
		if _, exists := serverNames[strings.ToLower(server.Name)]; exists {
			return Config{}, fmt.Errorf("netbox.servers contains duplicate name %q", server.Name)
		}
		serverNames[strings.ToLower(server.Name)] = struct{}{}
		if server.URL == "" {
			return Config{}, fmt.Errorf("netbox.servers[%d].url is required", index)
		}
		if !strings.HasPrefix(server.URL, "https://") && !strings.HasPrefix(server.URL, "http://") {
			return Config{}, fmt.Errorf("netbox.servers[%d].url must start with http:// or https://", index)
		}
	}

	ttl := 15 * time.Minute
	if raw.Cache.TTL != "" {
		ttl, err = time.ParseDuration(raw.Cache.TTL)
		if err != nil {
			return Config{}, fmt.Errorf("parse cache.ttl: %w", err)
		}
	}
	pingCount := raw.Ping.Count
	if pingCount == 0 {
		pingCount = 4
	}
	if pingCount < 1 {
		return Config{}, errors.New("ping.count must be greater than zero")
	}

	for user, key := range raw.SSH.Keys {
		key.IdentityFile, err = expandHome(key.IdentityFile)
		if err != nil {
			return Config{}, fmt.Errorf("expand ssh.keys.%s.identity_file: %w", user, err)
		}
		raw.SSH.Keys[user] = key
	}

	return Config{NetBox: NetBoxConfig{Servers: servers}, Services: raw.Services, SSH: raw.SSH, Cache: CacheConfig{TTL: ttl}, Ping: PingConfig{Count: pingCount}}, nil
}

func expandHome(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

func LoadCredentials(path string) (Credentials, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, fmt.Errorf("read credentials %q: %w", path, err)
	}

	var credentials Credentials
	if err := yaml.Unmarshal(contents, &credentials); err != nil {
		return Credentials{}, fmt.Errorf("parse credentials %q: %w", path, err)
	}
	if len(credentials.NetBox.Servers) == 0 {
		return Credentials{}, errors.New("netbox.servers is required in credentials")
	}
	return credentials, nil
}

func (credentials Credentials) TokenFor(serverName string) (string, error) {
	for name, server := range credentials.NetBox.Servers {
		if strings.EqualFold(strings.TrimSpace(name), serverName) && strings.TrimSpace(server.Token) != "" {
			return strings.TrimSpace(server.Token), nil
		}
	}
	return "", fmt.Errorf("netbox.servers.%s.token is required in credentials", serverName)
}
