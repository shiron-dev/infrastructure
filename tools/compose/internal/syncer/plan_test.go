package syncer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectLocalFiles(t *testing.T) {
	base := t.TempDir()

	// Create project files.
	projDir := filepath.Join(base, "projects", "grafana")
	os.MkdirAll(filepath.Join(projDir, "files", "provisioning"), 0755)
	os.WriteFile(filepath.Join(projDir, "compose.yml"), []byte("services: {}"), 0644)
	os.WriteFile(filepath.Join(projDir, "files", "grafana.ini"), []byte("[server]"), 0644)
	os.WriteFile(filepath.Join(projDir, "files", "provisioning", "ds.yml"), []byte("ds: 1"), 0644)

	// Create host project files.
	hostDir := filepath.Join(base, "hosts", "server1", "grafana")
	os.MkdirAll(filepath.Join(hostDir, "files"), 0755)
	os.WriteFile(filepath.Join(hostDir, "compose.override.yml"), []byte("override: true"), 0644)
	os.WriteFile(filepath.Join(hostDir, ".env"), []byte("GF_ADMIN=admin"), 0644)
	// This should override the project-level grafana.ini.
	os.WriteFile(filepath.Join(hostDir, "files", "grafana.ini"), []byte("[server]\nhost_override=true"), 0644)

	files, err := CollectLocalFiles(base, "server1", "grafana")
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]bool{
		"compose.yml":            true,
		"compose.override.yml":   true,
		".env":                   true,
		"grafana.ini":            true,
		"provisioning/ds.yml":    true,
	}

	if len(files) != len(expected) {
		t.Errorf("expected %d files, got %d: %v", len(expected), len(files), files)
	}

	for key := range expected {
		if _, ok := files[key]; !ok {
			t.Errorf("missing file %q", key)
		}
	}

	// Verify host override wins for grafana.ini.
	data, _ := os.ReadFile(files["grafana.ini"])
	if string(data) != "[server]\nhost_override=true" {
		t.Errorf("grafana.ini should be host version, got %q", string(data))
	}
}

func TestCollectLocalFiles_MissingProject(t *testing.T) {
	base := t.TempDir()
	os.MkdirAll(filepath.Join(base, "projects"), 0755)

	files, err := CollectLocalFiles(base, "server1", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestBuildManifest(t *testing.T) {
	files := map[string]string{
		"compose.yml": "/a/compose.yml",
		".env":        "/a/.env",
		"config.ini":  "/a/config.ini",
	}
	m := BuildManifest(files)
	if len(m.ManagedFiles) != 3 {
		t.Errorf("expected 3 managed files, got %d", len(m.ManagedFiles))
	}
	// Should be sorted.
	if m.ManagedFiles[0] != ".env" {
		t.Errorf("expected first file .env, got %q", m.ManagedFiles[0])
	}
}

func TestIsBinary(t *testing.T) {
	if isBinary([]byte("hello world")) {
		t.Error("text should not be binary")
	}
	if !isBinary([]byte("hello\x00world")) {
		t.Error("data with null byte should be binary")
	}
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
	}
	for _, tt := range tests {
		got := humanSize(tt.n)
		if got != tt.want {
			t.Errorf("humanSize(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestSyncPlanStats(t *testing.T) {
	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Projects: []ProjectPlan{
					{
						Files: []FilePlan{
							{Action: ActionAdd},
							{Action: ActionModify},
							{Action: ActionUnchanged},
						},
					},
					{
						Files: []FilePlan{
							{Action: ActionDelete},
							{Action: ActionAdd},
						},
					},
				},
			},
		},
	}

	hosts, projects, add, mod, del, unch := plan.Stats()
	if hosts != 1 {
		t.Errorf("hosts = %d", hosts)
	}
	if projects != 2 {
		t.Errorf("projects = %d", projects)
	}
	if add != 2 {
		t.Errorf("add = %d", add)
	}
	if mod != 1 {
		t.Errorf("mod = %d", mod)
	}
	if del != 1 {
		t.Errorf("del = %d", del)
	}
	if unch != 1 {
		t.Errorf("unch = %d", unch)
	}

	if !plan.HasChanges() {
		t.Error("plan should have changes")
	}
}
