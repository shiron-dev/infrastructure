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

	err := os.WriteFile(cfgPath, []byte(cfgContent), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// basePath が解決できるように compose ディレクトリを作成します。
	err = os.MkdirAll(filepath.Join(dir, "compose"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadCmtConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadCmtConfig: %v", err)
	}

	// basePath は絶対パスに解決されているはずです。
	if !filepath.IsAbs(cfg.BasePath) {
		t.Errorf("basePath should be absolute, got %q", cfg.BasePath)
	}

	if len(cfg.Hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(cfg.Hosts))
	}
	// ポートは設定レベルでは 0 がデフォルト。22 は後で ResolveSSHConfig により適用されます。
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

func TestLoadCmtConfig_WithBeforeApplyHooks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	cfgContent := `
basePath: ./compose
hosts:
  - name: server1
    host: 192.168.1.1
    user: deploy
beforeApplyHooks:
  beforePrompt:
    command: ./scripts/check-policy.sh
  afterPrompt:
    command: ./scripts/final-gate.sh
`

	cfgPath := filepath.Join(dir, "config.yml")

	err := os.WriteFile(cfgPath, []byte(cfgContent), 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(filepath.Join(dir, "compose"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadCmtConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadCmtConfig: %v", err)
	}

	if cfg.BeforeApplyHooks == nil {
		t.Fatal("beforeApplyHooks should not be nil")
	}

	if cfg.BeforeApplyHooks.BeforePrompt == nil {
		t.Fatal("beforePrompt should not be nil")
	}

	if cfg.BeforeApplyHooks.BeforePrompt.Command != "./scripts/check-policy.sh" {
		t.Errorf("beforePrompt.command = %q", cfg.BeforeApplyHooks.BeforePrompt.Command)
	}

	if cfg.BeforeApplyHooks.AfterPrompt == nil {
		t.Fatal("afterPrompt should not be nil")
	}

	if cfg.BeforeApplyHooks.AfterPrompt.Command != "./scripts/final-gate.sh" {
		t.Errorf("afterPrompt.command = %q", cfg.BeforeApplyHooks.AfterPrompt.Command)
	}
}

func TestLoadCmtConfig_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "missing basePath",
			content: "hosts:\n  - name: x\n    host: x\n    user: x\n",
			wantErr: true,
		},
		{
			name:    "no hosts",
			content: "basePath: .\nhosts: []\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "bad.yml")

			err := os.WriteFile(cfgPath, []byte(tt.content), 0600)
			if err != nil {
				t.Fatal(err)
			}

			_, err = LoadCmtConfig(cfgPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadCmtConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDiscoverProjects(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	projDir := filepath.Join(dir, "projects")

	err := os.MkdirAll(filepath.Join(projDir, "grafana"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(filepath.Join(projDir, "prometheus"), 0750)
	if err != nil {
		t.Fatal(err)
	}
	// 通常ファイルは無視されるべきです。
	err = os.WriteFile(filepath.Join(projDir, "README.md"), []byte("hi"), 0600)
	if err != nil {
		t.Fatal(err)
	}

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

	tests := []struct {
		name      string
		filter    []string
		wantCount int
		wantNames []string
	}{
		{
			name:      "nil filter returns all",
			filter:    nil,
			wantCount: 3,
			wantNames: []string{"a", "b", "c"},
		},
		{
			name:      "specific filter",
			filter:    []string{"a", "c"},
			wantCount: 2,
			wantNames: []string{"a", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FilterHosts(hosts, tt.filter)
			if len(got) != tt.wantCount {
				t.Errorf("FilterHosts() count = %d, want %d", len(got), tt.wantCount)
			}

			for i, name := range tt.wantNames {
				if i >= len(got) || got[i].Name != name {
					t.Errorf("FilterHosts() hosts = %v, want names %v", got, tt.wantNames)

					break
				}
			}
		})
	}
}

func TestFilterProjects(t *testing.T) {
	t.Parallel()

	projects := []string{"grafana", "prometheus", "loki"}

	tests := []struct {
		name   string
		filter []string
		want   []string
	}{
		{
			name:   "nil filter returns all",
			filter: nil,
			want:   []string{"grafana", "prometheus", "loki"},
		},
		{
			name:   "specific filter",
			filter: []string{"grafana"},
			want:   []string{"grafana"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FilterProjects(projects, tt.filter)
			if len(got) != len(tt.want) {
				t.Errorf("FilterProjects() = %v, want %v", got, tt.want)

				return
			}

			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("FilterProjects() = %v, want %v", got, tt.want)

					break
				}
			}
		})
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

	tests := []struct {
		name            string
		hostCfg         *HostConfig
		project         string
		wantRemotePath  string
		wantPostCommand string
	}{
		{
			name:            "layer 1 only",
			hostCfg:         nil,
			project:         "grafana",
			wantRemotePath:  "/opt/default",
			wantPostCommand: "echo default",
		},
		{
			name:            "layer 2 overrides path, layer 1 provides command",
			hostCfg:         hostCfg,
			project:         "prometheus",
			wantRemotePath:  "/opt/host",
			wantPostCommand: "echo default",
		},
		{
			name:            "layer 3 overrides command",
			hostCfg:         hostCfg,
			project:         "grafana",
			wantRemotePath:  "/opt/host",
			wantPostCommand: "docker compose up -d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolved := ResolveProjectConfig(cmtDefaults, tt.hostCfg, tt.project)
			if resolved.RemotePath != tt.wantRemotePath {
				t.Errorf("RemotePath = %q, want %q", resolved.RemotePath, tt.wantRemotePath)
			}

			if resolved.PostSyncCommand != tt.wantPostCommand {
				t.Errorf("PostSyncCommand = %q, want %q", resolved.PostSyncCommand, tt.wantPostCommand)
			}
		})
	}
}

func TestResolveProjectConfig_ComposeAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		defaults   *SyncDefaults
		hostCfg    *HostConfig
		project    string
		wantAction string
	}{
		{
			name:       "defaults to up when unset",
			defaults:   &SyncDefaults{RemotePath: "/opt"},
			hostCfg:    nil,
			project:    "grafana",
			wantAction: ComposeActionUp,
		},
		{
			name:       "defaults level sets action",
			defaults:   &SyncDefaults{RemotePath: "/opt", ComposeAction: ComposeActionDown},
			hostCfg:    nil,
			project:    "grafana",
			wantAction: ComposeActionDown,
		},
		{
			name:     "host level overrides defaults",
			defaults: &SyncDefaults{RemotePath: "/opt", ComposeAction: ComposeActionUp},
			hostCfg: &HostConfig{
				ComposeAction: ComposeActionDown,
			},
			project:    "grafana",
			wantAction: ComposeActionDown,
		},
		{
			name:     "project level overrides host",
			defaults: &SyncDefaults{RemotePath: "/opt"},
			hostCfg: &HostConfig{
				ComposeAction: ComposeActionUp,
				Projects: map[string]*ProjectConfig{
					"grafana": {ComposeAction: ComposeActionDown},
				},
			},
			project:    "grafana",
			wantAction: ComposeActionDown,
		},
		{
			name:     "unset project inherits host",
			defaults: &SyncDefaults{RemotePath: "/opt"},
			hostCfg: &HostConfig{
				ComposeAction: ComposeActionDown,
				Projects: map[string]*ProjectConfig{
					"grafana": {},
				},
			},
			project:    "grafana",
			wantAction: ComposeActionDown,
		},
		{
			name:     "project can ignore compose runtime state",
			defaults: &SyncDefaults{RemotePath: "/opt", ComposeAction: ComposeActionUp},
			hostCfg: &HostConfig{
				Projects: map[string]*ProjectConfig{
					"grafana": {ComposeAction: ComposeActionIgnore},
				},
			},
			project:    "grafana",
			wantAction: ComposeActionIgnore,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolved := ResolveProjectConfig(tt.defaults, tt.hostCfg, tt.project)
			if resolved.ComposeAction != tt.wantAction {
				t.Errorf("ComposeAction = %q, want %q", resolved.ComposeAction, tt.wantAction)
			}
		})
	}
}

func TestResolveProjectConfig_RemoveOrphans(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		defaults          *SyncDefaults
		hostCfg           *HostConfig
		project           string
		wantRemoveOrphans bool
	}{
		{
			name:              "defaults to false when unset",
			defaults:          &SyncDefaults{RemotePath: "/opt"},
			hostCfg:           nil,
			project:           "grafana",
			wantRemoveOrphans: false,
		},
		{
			name:     "project level enables remove-orphans",
			defaults: &SyncDefaults{RemotePath: "/opt"},
			hostCfg: &HostConfig{
				Projects: map[string]*ProjectConfig{
					"grafana": {RemoveOrphans: true},
				},
			},
			project:           "grafana",
			wantRemoveOrphans: true,
		},
		{
			name:     "missing project keeps false",
			defaults: &SyncDefaults{RemotePath: "/opt"},
			hostCfg: &HostConfig{
				Projects: map[string]*ProjectConfig{
					"prometheus": {RemoveOrphans: true},
				},
			},
			project:           "grafana",
			wantRemoveOrphans: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolved := ResolveProjectConfig(tt.defaults, tt.hostCfg, tt.project)
			if resolved.RemoveOrphans != tt.wantRemoveOrphans {
				t.Errorf("RemoveOrphans = %v, want %v", resolved.RemoveOrphans, tt.wantRemoveOrphans)
			}
		})
	}
}

func TestLoadHostConfig_ComposeAction(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hostDir := filepath.Join(dir, "hosts", "server1")

	err := os.MkdirAll(hostDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	content := `
remotePath: /srv/compose
composeAction: down
projects:
  grafana:
    composeAction: up
    removeOrphans: true
`

	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}

	hostConfig, err := LoadHostConfig(dir, "server1")
	if err != nil {
		t.Fatal(err)
	}

	if hostConfig.ComposeAction != "down" {
		t.Errorf("host composeAction = %q, want %q", hostConfig.ComposeAction, "down")
	}

	if hostConfig.Projects["grafana"].ComposeAction != "up" {
		t.Errorf("grafana composeAction = %q, want %q", hostConfig.Projects["grafana"].ComposeAction, "up")
	}

	if !hostConfig.Projects["grafana"].RemoveOrphans {
		t.Error("grafana removeOrphans should be true")
	}
}

func TestLoadHostConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// host.yml が無い場合 → nil, nil。
	hostConfig, err := LoadHostConfig(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected not-found error for missing host.yml")
	}

	if hostConfig != nil {
		t.Error("expected nil for missing host.yml")
	}

	// 有効な host.yml。
	hostDir := filepath.Join(dir, "hosts", "server1")

	err = os.MkdirAll(hostDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	content := `
remotePath: /srv/compose
postSyncCommand: docker compose up -d
projects:
  grafana:
    postSyncCommand: docker compose -f compose.yml -f compose.override.yml up -d
`

	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}

	hostConfig, err = LoadHostConfig(dir, "server1")
	if err != nil {
		t.Fatal(err)
	}

	if hostConfig.RemotePath != "/srv/compose" {
		t.Errorf("remotePath = %q", hostConfig.RemotePath)
	}

	if hostConfig.Projects["grafana"] == nil {
		t.Fatal("grafana project config missing")
	}
}
