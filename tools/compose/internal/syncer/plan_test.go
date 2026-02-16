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

	// grafana.ini に対してホストの上書きが勝つことを確認します。
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
	// ソートされているべきです。
	if manifest.ManagedFiles[0] != ".env" {
		t.Errorf("expected first file .env, got %q", manifest.ManagedFiles[0])
	}
}

func TestIsBinary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "text should not be binary",
			data: []byte("hello world"),
			want: false,
		},
		{
			name: "data with null byte should be binary",
			data: []byte("hello\x00world"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isBinary(tt.data)
			if got != tt.want {
				t.Errorf("isBinary() = %v, want %v", got, tt.want)
			}
		})
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

	tests := []struct {
		name         string
		filename     string
		remote       []byte
		local        []byte
		wantEmpty    bool
		wantContains []string
	}{
		{
			name:      "identical content",
			filename:  "file.txt",
			remote:    []byte("hello\n"),
			local:     []byte("hello\n"),
			wantEmpty: true,
		},
		{
			name:     "basic diff",
			filename: "compose.yml",
			remote:   []byte("line1\nline2\nline3\n"),
			local:    []byte("line1\nmodified\nline3\n"),
			wantContains: []string{
				"compose.yml (remote)",
				"compose.yml (local)",
				"-line2",
				"+modified",
			},
		},
		{
			name:     "added lines",
			filename: "test.txt",
			remote:   []byte("a\n"),
			local:    []byte("a\nb\nc\n"),
			wantContains: []string{
				"+b",
				"+c",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := computeDiff(tt.filename, tt.remote, tt.local)

			if tt.wantEmpty {
				if result != "" {
					t.Errorf("expected empty diff, got %q", result)
				}

				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("diff should contain %q, got:\n%s", want, result)
				}
			}
		})
	}
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

func TestHasChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		plan        *SyncPlan
		wantChanges bool
	}{
		{
			name: "no changes",
			plan: &SyncPlan{
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
			},
			wantChanges: false,
		},
		{
			name: "dir creation",
			plan: &SyncPlan{
				HostPlans: []HostPlan{
					{
						Projects: []ProjectPlan{
							{
								Files: []FilePlan{
									{Action: ActionUnchanged},
								},
								Dirs: []DirPlan{
									{Exists: false},
								},
							},
						},
					},
				},
			},
			wantChanges: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.plan.HasChanges()
			if got != tt.wantChanges {
				t.Errorf("HasChanges() = %v, want %v", got, tt.wantChanges)
			}
		})
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

	// ホストヘッダーをチェックします。
	if !strings.Contains(output, "Host: server1") {
		t.Error("output should contain host name")
	}

	if !strings.Contains(output, "deploy@192.168.1.1:22") {
		t.Error("output should contain connection info")
	}

	// プロジェクト情報をチェックします。
	if !strings.Contains(output, "Project: grafana") {
		t.Error("output should contain project name")
	}

	if !strings.Contains(output, "Post-sync:") {
		t.Error("output should contain post-sync command")
	}

	// ディレクトリプランをチェックします。
	if !strings.Contains(output, "+ data/") {
		t.Error("output should show dir to create")
	}

	// ファイルプランをチェックします。
	if !strings.Contains(output, "+ compose.yml") {
		t.Error("output should show added file")
	}

	if !strings.Contains(output, "= .env") {
		t.Error("output should show unchanged file")
	}

	// サマリーをチェックします。
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
