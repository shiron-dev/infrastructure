package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCmtConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	cfgContent := `
basePath: ./compose
hosts:
  - name: server1
    host: 192.168.1.1
    user: deploy
    sshAgent: true
  - name: server2
    host: 192.168.1.2
    port: 2222
    user: deploy
    sshKeyPath: /home/deploy/.ssh/id_ed25519
defaults:
  remotePath: /opt/compose
  postSyncCommand: docker compose up -d
`

	cfgPath := filepath.Join(dir, "config.yml")

	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create the compose directory so basePath resolves.
	if err := os.MkdirAll(filepath.Join(dir, "compose"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadCmtConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadCmtConfig: %v", err)
	}

	// basePath should be resolved to an absolute path.
	if !filepath.IsAbs(cfg.BasePath) {
		t.Errorf("basePath should be absolute, got %q", cfg.BasePath)
	}

	if len(cfg.Hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(cfg.Hosts))
	}
	// Port defaults to 0 at config level; 22 is applied later by ResolveSSHConfig.
	if cfg.Hosts[0].Port != 0 {
		t.Errorf("default port should be 0 (unset), got %d", cfg.Hosts[0].Port)
	}

	if cfg.Hosts[1].Port != 2222 {
		t.Errorf("server2 port should be 2222, got %d", cfg.Hosts[1].Port)
	}

	if cfg.Defaults == nil {
		t.Fatal("defaults should not be nil")
	}

	if cfg.Defaults.RemotePath != "/opt/compose" {
		t.Errorf("remotePath = %q", cfg.Defaults.RemotePath)
	}
}

func TestLoadCmtConfig_Errors(t *testing.T) {
	t.Parallel()

	t.Run("missing basePath", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "bad.yml")
		os.WriteFile(cfgPath, []byte("hosts:\n  - name: x\n    host: x\n    user: x\n"), 0644)

		_, err := LoadCmtConfig(cfgPath)
		if err == nil {
			t.Error("expected error for missing basePath")
		}
	})

	t.Run("no hosts", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "bad.yml")
		os.WriteFile(cfgPath, []byte("basePath: .\nhosts: []\n"), 0644)

		_, err := LoadCmtConfig(cfgPath)
		if err == nil {
			t.Error("expected error for empty hosts")
		}
	})
}

func TestDiscoverProjects(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	projDir := filepath.Join(dir, "projects")
	os.MkdirAll(filepath.Join(projDir, "grafana"), 0755)
	os.MkdirAll(filepath.Join(projDir, "prometheus"), 0755)
	// A regular file should be ignored.
	os.WriteFile(filepath.Join(projDir, "README.md"), []byte("hi"), 0644)

	projects, err := DiscoverProjects(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d: %v", len(projects), projects)
	}
}

func TestFilterHosts(t *testing.T) {
	t.Parallel()

	hosts := []HostEntry{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	}

	// Empty filter returns all.
	if got := FilterHosts(hosts, nil); len(got) != 3 {
		t.Errorf("nil filter: got %d hosts", len(got))
	}

	// Specific filter.
	got := FilterHosts(hosts, []string{"a", "c"})
	if len(got) != 2 {
		t.Errorf("expected 2, got %d", len(got))
	}

	if got[0].Name != "a" || got[1].Name != "c" {
		t.Errorf("unexpected hosts: %v", got)
	}
}

func TestFilterProjects(t *testing.T) {
	t.Parallel()

	projects := []string{"grafana", "prometheus", "loki"}

	if got := FilterProjects(projects, nil); len(got) != 3 {
		t.Errorf("nil filter: got %d", len(got))
	}

	got := FilterProjects(projects, []string{"grafana"})
	if len(got) != 1 || got[0] != "grafana" {
		t.Errorf("unexpected: %v", got)
	}
}

func TestResolveProjectConfig(t *testing.T) {
	t.Parallel()

	cmtDefaults := &SyncDefaults{
		RemotePath:      "/opt/default",
		PostSyncCommand: "echo default",
	}
	hostCfg := &HostConfig{
		RemotePath:      "/opt/host",
		PostSyncCommand: "",
		Projects: map[string]*ProjectConfig{
			"grafana": {
				PostSyncCommand: "docker compose up -d",
			},
		},
	}

	// Layer 1 only.
	r := ResolveProjectConfig(cmtDefaults, nil, "grafana")
	if r.RemotePath != "/opt/default" {
		t.Errorf("expected /opt/default, got %q", r.RemotePath)
	}

	// Layer 2 overrides path, layer 1 provides command.
	r = ResolveProjectConfig(cmtDefaults, hostCfg, "prometheus")
	if r.RemotePath != "/opt/host" {
		t.Errorf("expected /opt/host, got %q", r.RemotePath)
	}

	if r.PostSyncCommand != "echo default" {
		t.Errorf("expected echo default, got %q", r.PostSyncCommand)
	}

	// Layer 3 overrides command.
	r = ResolveProjectConfig(cmtDefaults, hostCfg, "grafana")
	if r.RemotePath != "/opt/host" {
		t.Errorf("expected /opt/host, got %q", r.RemotePath)
	}

	if r.PostSyncCommand != "docker compose up -d" {
		t.Errorf("expected docker compose up -d, got %q", r.PostSyncCommand)
	}
}

func TestLoadHostConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// No host.yml → nil, nil.
	hc, err := LoadHostConfig(dir, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}

	if hc != nil {
		t.Error("expected nil for missing host.yml")
	}

	// Valid host.yml.
	hostDir := filepath.Join(dir, "hosts", "server1")
	os.MkdirAll(hostDir, 0755)

	content := `
remotePath: /srv/compose
postSyncCommand: docker compose up -d
projects:
  grafana:
    postSyncCommand: docker compose -f compose.yml -f compose.override.yml up -d
`
	os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte(content), 0644)

	hc, err = LoadHostConfig(dir, "server1")
	if err != nil {
		t.Fatal(err)
	}

	if hc.RemotePath != "/srv/compose" {
		t.Errorf("remotePath = %q", hc.RemotePath)
	}

	if hc.Projects["grafana"] == nil {
		t.Fatal("grafana project config missing")
	}
}
