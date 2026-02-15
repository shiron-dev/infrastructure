package syncer

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cmt/internal/config"
	"cmt/internal/remote"

	"go.uber.org/mock/gomock"
)

func TestCollectLocalFiles(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	setupCollectLocalFilesFixture(t, base)

	files, err := CollectLocalFiles(base, "server1", "grafana")
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"compose.yml",
		"compose.override.yml",
		".env",
		"grafana.ini",
		"provisioning/ds.yml",
	}

	assertContainsFiles(t, files, expected)

	// Verify host override wins for grafana.ini.
	data, _ := os.ReadFile(files["grafana.ini"])
	if string(data) != "[server]\nhost_override=true" {
		t.Errorf("grafana.ini should be host version, got %q", string(data))
	}
}

func setupCollectLocalFilesFixture(t *testing.T, base string) {
	t.Helper()

	projDir := filepath.Join(base, "projects", "grafana")
	mustMkdirAll(t, filepath.Join(projDir, "files", "provisioning"))
	mustWriteFile(t, filepath.Join(projDir, "compose.yml"), []byte("services: {}"))
	mustWriteFile(t, filepath.Join(projDir, "files", "grafana.ini"), []byte("[server]"))
	mustWriteFile(t, filepath.Join(projDir, "files", "provisioning", "ds.yml"), []byte("ds: 1"))

	hostDir := filepath.Join(base, "hosts", "server1", "grafana")
	mustMkdirAll(t, filepath.Join(hostDir, "files"))
	mustWriteFile(t, filepath.Join(hostDir, "compose.override.yml"), []byte("override: true"))
	mustWriteFile(t, filepath.Join(hostDir, ".env"), []byte("GF_ADMIN=admin"))
	mustWriteFile(t, filepath.Join(hostDir, "files", "grafana.ini"), []byte("[server]\nhost_override=true"))
}

func assertContainsFiles(t *testing.T, got map[string]string, expected []string) {
	t.Helper()

	if len(got) != len(expected) {
		t.Errorf("expected %d files, got %d: %v", len(expected), len(got), got)
	}

	for _, key := range expected {
		if _, ok := got[key]; !ok {
			t.Errorf("missing file %q", key)
		}
	}
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()

	err := os.MkdirAll(dir, 0750)
	if err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, filePath string, content []byte) {
	t.Helper()

	err := os.WriteFile(filePath, content, 0600)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCollectLocalFiles_MissingProject(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	err := os.MkdirAll(filepath.Join(base, "projects"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	files, err := CollectLocalFiles(base, "server1", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestBuildManifest(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"compose.yml": "/a/compose.yml",
		".env":        "/a/.env",
		"config.ini":  "/a/config.ini",
	}

	manifest := BuildManifest(files)
	if len(manifest.ManagedFiles) != 3 {
		t.Errorf("expected 3 managed files, got %d", len(manifest.ManagedFiles))
	}
	// Should be sorted.
	if manifest.ManagedFiles[0] != ".env" {
		t.Errorf("expected first file .env, got %q", manifest.ManagedFiles[0])
	}
}

func TestIsBinary(t *testing.T) {
	t.Parallel()

	if isBinary([]byte("hello world")) {
		t.Error("text should not be binary")
	}

	if !isBinary([]byte("hello\x00world")) {
		t.Error("data with null byte should be binary")
	}
}

func TestHumanSize(t *testing.T) {
	t.Parallel()

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
	for _, testCase := range tests {
		t.Run(testCase.want, func(t *testing.T) {
			t.Parallel()

			got := humanSize(testCase.n)
			if got != testCase.want {
				t.Errorf("humanSize(%d) = %q, want %q", testCase.n, got, testCase.want)
			}
		})
	}
}

func TestSyncPlanStats(t *testing.T) {
	t.Parallel()

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

// ---------------------------------------------------------------------------
// ActionType helpers
// ---------------------------------------------------------------------------

func TestActionType_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		action ActionType
		want   string
	}{
		{ActionUnchanged, "unchanged"},
		{ActionAdd, "add"},
		{ActionModify, "modify"},
		{ActionDelete, "delete"},
	}
	for _, testCase := range tests {
		t.Run(testCase.want, func(t *testing.T) {
			t.Parallel()

			if got := testCase.action.String(); got != testCase.want {
				t.Errorf("String() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestActionType_Symbol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		action ActionType
		want   string
	}{
		{ActionUnchanged, "="},
		{ActionAdd, "+"},
		{ActionModify, "~"},
		{ActionDelete, "-"},
	}
	for _, testCase := range tests {
		t.Run(testCase.want, func(t *testing.T) {
			t.Parallel()

			if got := testCase.action.Symbol(); got != testCase.want {
				t.Errorf("Symbol() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// computeDiff
// ---------------------------------------------------------------------------

func TestComputeDiff(t *testing.T) {
	t.Parallel()

	t.Run("identical content", func(t *testing.T) {
		t.Parallel()

		result := computeDiff("file.txt", []byte("hello\n"), []byte("hello\n"))
		if result != "" {
			t.Errorf("expected empty diff for identical content, got %q", result)
		}
	})

	t.Run("basic diff", func(t *testing.T) {
		t.Parallel()

		remote := []byte("line1\nline2\nline3\n")
		local := []byte("line1\nmodified\nline3\n")

		result := computeDiff("compose.yml", remote, local)

		if !strings.Contains(result, "compose.yml (remote)") {
			t.Error("diff should contain remote file header")
		}

		if !strings.Contains(result, "compose.yml (local)") {
			t.Error("diff should contain local file header")
		}

		if !strings.Contains(result, "-line2") {
			t.Error("diff should contain removed line")
		}

		if !strings.Contains(result, "+modified") {
			t.Error("diff should contain added line")
		}
	})

	t.Run("added lines", func(t *testing.T) {
		t.Parallel()

		remote := []byte("a\n")
		local := []byte("a\nb\nc\n")

		result := computeDiff("test.txt", remote, local)

		if !strings.Contains(result, "+b") {
			t.Error("diff should contain added line b")
		}

		if !strings.Contains(result, "+c") {
			t.Error("diff should contain added line c")
		}
	})
}

// ---------------------------------------------------------------------------
// DirStats / HasChanges
// ---------------------------------------------------------------------------

func TestDirStats(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Projects: []ProjectPlan{
					{
						Dirs: []DirPlan{
							{RelativePath: "data", Exists: false},
							{RelativePath: "logs", Exists: true},
							{RelativePath: "config", Exists: false},
						},
					},
				},
			},
		},
	}

	toCreate, existing := plan.DirStats()
	if toCreate != 2 {
		t.Errorf("toCreate = %d, want 2", toCreate)
	}

	if existing != 1 {
		t.Errorf("existing = %d, want 1", existing)
	}
}

func TestHasChanges_NoChanges(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Projects: []ProjectPlan{
					{
						Files: []FilePlan{
							{Action: ActionUnchanged},
							{Action: ActionUnchanged},
						},
						Dirs: []DirPlan{
							{Exists: true},
						},
					},
				},
			},
		},
	}

	if plan.HasChanges() {
		t.Error("plan with only unchanged files and existing dirs should not have changes")
	}
}

func TestHasChanges_DirCreation(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Projects: []ProjectPlan{
					{
						Files: []FilePlan{
							{Action: ActionUnchanged},
						},
						Dirs: []DirPlan{
							{Exists: false}, // needs creation
						},
					},
				},
			},
		},
	}

	if !plan.HasChanges() {
		t.Error("plan with dir to create should have changes")
	}
}

// ---------------------------------------------------------------------------
// SyncPlan.Print
// ---------------------------------------------------------------------------

func TestSyncPlan_Print_NoHosts(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{}

	var buf bytes.Buffer

	plan.Print(&buf)

	if !strings.Contains(buf.String(), "No hosts selected") {
		t.Errorf("expected 'No hosts selected', got %q", buf.String())
	}
}

func TestSyncPlan_Print_FullPlan(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{
					Name: "server1",
					User: "deploy",
					Host: "192.168.1.1",
					Port: 22,
				},
				Projects: []ProjectPlan{
					{
						ProjectName:     "grafana",
						RemoteDir:       "/opt/compose/grafana",
						PostSyncCommand: "docker compose up -d",
						Dirs: []DirPlan{
							{RelativePath: "data", Exists: false},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								Action:       ActionAdd,
								LocalData:    []byte("services: {}"),
							},
							{
								RelativePath: ".env",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer

	plan.Print(&buf)
	output := buf.String()

	// Check host header.
	if !strings.Contains(output, "Host: server1") {
		t.Error("output should contain host name")
	}

	if !strings.Contains(output, "deploy@192.168.1.1:22") {
		t.Error("output should contain connection info")
	}

	// Check project info.
	if !strings.Contains(output, "Project: grafana") {
		t.Error("output should contain project name")
	}

	if !strings.Contains(output, "Post-sync:") {
		t.Error("output should contain post-sync command")
	}

	// Check dir plan.
	if !strings.Contains(output, "+ data/") {
		t.Error("output should show dir to create")
	}

	// Check file plans.
	if !strings.Contains(output, "+ compose.yml") {
		t.Error("output should show added file")
	}

	if !strings.Contains(output, "= .env") {
		t.Error("output should show unchanged file")
	}

	// Check summary.
	if !strings.Contains(output, "Summary:") {
		t.Error("output should contain summary")
	}

	if !strings.Contains(output, "1 to add") {
		t.Error("summary should show add count")
	}
}

func TestBuildPlanWithDeps_UsesInjectedDependencies(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	projectDir := filepath.Join(base, "projects", "grafana")

	err := os.MkdirAll(projectDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(projectDir, "compose.yml"), []byte("services: {}"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(filepath.Join(base, "hosts", "server1", "grafana"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.CmtConfig{
		BasePath: base,
		Defaults: &config.SyncDefaults{RemotePath: "/srv/compose"},
		Hosts: []config.HostEntry{
			{Name: "server1", Host: "server1-alias", User: "deploy"},
		},
	}

	ctrl := gomock.NewController(t)
	resolver := config.NewMockSSHConfigResolver(ctrl)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	hostDir := filepath.Join(base, "hosts", "server1")
	gomock.InOrder(
		resolver.EXPECT().
			Resolve(gomock.Any(), "", hostDir).
			DoAndReturn(func(entry *config.HostEntry, _, _ string) error {
				entry.Host = "10.0.0.10"
				entry.Port = 22

				return nil
			}),
		factory.EXPECT().
			NewClient(gomock.AssignableToTypeOf(config.HostEntry{})).
			Return(client, nil),
	)
	client.EXPECT().
		ReadFile("/srv/compose/grafana/.cmt-manifest.json").
		Return(nil, errors.New("manifest not found"))
	client.EXPECT().
		ReadFile("/srv/compose/grafana/compose.yml").
		Return(nil, errors.New("remote file missing"))
	client.EXPECT().Close().Return(nil)

	plan, err := BuildPlanWithDeps(cfg, nil, nil, PlanDependencies{
		ClientFactory: factory,
		SSHResolver:   resolver,
	})
	if err != nil {
		t.Fatalf("BuildPlanWithDeps: %v", err)
	}

	if len(plan.HostPlans) != 1 {
		t.Fatalf("host plans = %d, want 1", len(plan.HostPlans))
	}

	if len(plan.HostPlans[0].Projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(plan.HostPlans[0].Projects))
	}

	files := plan.HostPlans[0].Projects[0].Files
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}

	if files[0].Action != ActionAdd {
		t.Fatalf("file action = %v, want %v", files[0].Action, ActionAdd)
	}
}
