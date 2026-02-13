package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadCmtConfig reads and parses the cmt configuration file.
// Relative basePath values are resolved relative to the config file location.
func LoadCmtConfig(configPath string) (*CmtConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", configPath, err)
	}

	var cfg CmtConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", configPath, err)
	}

	if cfg.BasePath == "" {
		return nil, fmt.Errorf("basePath is required in %s", configPath)
	}

	if len(cfg.Hosts) == 0 {
		return nil, fmt.Errorf("at least one host is required in %s", configPath)
	}

	// Resolve relative basePath against the config file's directory.
	if !filepath.IsAbs(cfg.BasePath) {
		configDir := filepath.Dir(configPath)

		abs, err := filepath.Abs(filepath.Join(configDir, cfg.BasePath))
		if err != nil {
			return nil, fmt.Errorf("resolving basePath: %w", err)
		}

		cfg.BasePath = abs
	}

	return &cfg, nil
}

// LoadHostConfig reads host.yml for the given host name.
// Returns nil (without error) when the file does not exist.
func LoadHostConfig(basePath, hostName string) (*HostConfig, error) {
	p := filepath.Join(basePath, "hosts", hostName, "host.yml")

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("reading %s: %w", p, err)
	}

	var hc HostConfig
	if err := yaml.Unmarshal(data, &hc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", p, err)
	}

	return &hc, nil
}

// DiscoverProjects lists project names found under basePath/projects/.
func DiscoverProjects(basePath string) ([]string, error) {
	dir := filepath.Join(basePath, "projects")

	entries, err := os.ReadDir(dir)

	if err != nil {
		return nil, fmt.Errorf("reading projects directory %s: %w", dir, err)
	}

	var projects []string

	for _, e := range entries {
		if e.IsDir() {
			projects = append(projects, e.Name())
		}
	}

	return projects, nil
}

// FilterHosts returns only the hosts whose names appear in the filter list.
// If filter is empty, all hosts are returned.
func FilterHosts(hosts []HostEntry, filter []string) []HostEntry {
	if len(filter) == 0 {
		return hosts
	}

	set := make(map[string]bool, len(filter))
	for _, f := range filter {
		set[f] = true
	}

	var out []HostEntry

	for _, h := range hosts {
		if set[h.Name] {
			out = append(out, h)
		}
	}

	return out
}

// FilterProjects returns only the project names that appear in the filter list.
// If filter is empty, all projects are returned.
func FilterProjects(projects []string, filter []string) []string {
	if len(filter) == 0 {
		return projects
	}

	set := make(map[string]bool, len(filter))
	for _, f := range filter {
		set[f] = true
	}

	var out []string

	for _, p := range projects {
		if set[p] {
			out = append(out, p)
		}
	}

	return out
}
