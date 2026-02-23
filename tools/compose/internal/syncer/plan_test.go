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

type mockLocalCommandRunner struct {
	run func(name string, args []string, workdir string) (string, error)
}

func (m mockLocalCommandRunner) Run(name string, args []string, workdir string) (string, error) {
	return m.run(name, args, workdir)
}

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
	client.EXPECT().
		RunCommand("/srv/compose/grafana", "docker compose config --services 2>/dev/null").
		Return("", errors.New("not found"))
	client.EXPECT().
		RunCommand("/srv/compose/grafana", "docker compose ps --services --filter status=running 2>/dev/null").
		Return("", errors.New("not found"))
	client.EXPECT().Close().Return(nil)

	runner := mockLocalCommandRunner{
		run: func(name string, args []string, _ string) (string, error) {
			if name != "docker" {
				t.Fatalf("name = %q, want docker", name)
			}

			wantArgs := []string{"compose", "-f", "compose.yml", "config"}
			if len(args) != len(wantArgs) {
				t.Fatalf("args len = %d, want %d; args = %v", len(args), len(wantArgs), args)
			}

			for i := range wantArgs {
				if args[i] != wantArgs[i] {
					t.Fatalf("args[%d] = %q, want %q", i, args[i], wantArgs[i])
				}
			}

			return "ok", nil
		},
	}

	plan, err := BuildPlanWithDeps(cfg, nil, nil, PlanDependencies{
		ClientFactory: factory,
		SSHResolver:   resolver,
		LocalRunner:   runner,
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

func TestBuildPlanWithDeps_ComposeValidationFails(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	projectDir := filepath.Join(base, "projects", "grafana")

	err := os.MkdirAll(projectDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(projectDir, "compose.yml"), []byte("services: {}"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(filepath.Join(base, "hosts", "server1", "grafana"), 0o750)
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
		resolver.EXPECT().Resolve(gomock.Any(), "", hostDir).Return(nil),
		factory.EXPECT().NewClient(gomock.AssignableToTypeOf(config.HostEntry{})).Return(client, nil),
	)
	client.EXPECT().
		ReadFile("/srv/compose/grafana/.cmt-manifest.json").
		Return(nil, errors.New("manifest not found"))
	client.EXPECT().
		ReadFile("/srv/compose/grafana/compose.yml").
		Return(nil, errors.New("remote file missing"))
	client.EXPECT().Close().Return(nil)

	_, err = BuildPlanWithDeps(cfg, nil, nil, PlanDependencies{
		ClientFactory: factory,
		SSHResolver:   resolver,
		LocalRunner: mockLocalCommandRunner{
			run: func(string, []string, string) (string, error) {
				return "compose is invalid", errors.New("exit status 1")
			},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "validating docker compose config for server1/grafana failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Pattern-based diff masking
// ---------------------------------------------------------------------------

func TestBuildDiffMaskPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		vars    map[string]any
		wantLen int
		wantPfx []string
		wantSfx []string
	}{
		{
			name:    "env var style",
			raw:     "DB_HOST={{ .db_host }}\nDB_PORT={{ .db_port }}\n",
			vars:    map[string]any{"db_host": "pg.local", "db_port": 5432},
			wantLen: 2,
			wantPfx: []string{"DB_HOST=", "DB_PORT="},
			wantSfx: []string{"", ""},
		},
		{
			name:    "value with surrounding text",
			raw:     `password = """{{ .pw }}"""` + "\n",
			vars:    map[string]any{"pw": "s3cret"},
			wantLen: 1,
			wantPfx: []string{`password = """`},
			wantSfx: []string{`"""`},
		},
		{
			name:    "no vars returns nil",
			raw:     "plain line\n",
			vars:    nil,
			wantLen: 0,
		},
		{
			name:    "no templates returns nil",
			raw:     "static content\n",
			vars:    map[string]any{"key": "val"},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rawData := []byte(tt.raw)

			rendered, err := RenderTemplate(rawData, tt.vars)
			if err != nil && tt.wantLen > 0 {
				t.Fatalf("render error: %v", err)
			}

			if tt.vars == nil {
				rendered = rawData
			}

			patterns := buildDiffMaskPatterns(rawData, rendered, tt.vars)
			if len(patterns) != tt.wantLen {
				t.Fatalf("len = %d, want %d; patterns = %+v", len(patterns), tt.wantLen, patterns)
			}

			for i, pat := range patterns {
				if i < len(tt.wantPfx) && pat.prefix != tt.wantPfx[i] {
					t.Errorf("pattern[%d].prefix = %q, want %q", i, pat.prefix, tt.wantPfx[i])
				}

				if i < len(tt.wantSfx) && pat.suffix != tt.wantSfx[i] {
					t.Errorf("pattern[%d].suffix = %q, want %q", i, pat.suffix, tt.wantSfx[i])
				}
			}
		})
	}
}

func TestMaskDiffWithPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		diff        string
		patterns    []maskPattern
		wantContain []string
		wantAbsent  []string
	}{
		{
			name: "masks + and - lines matching pattern",
			diff: "--- f (remote)\n+++ f (local)\n@@ -1,3 +1,3 @@\n" +
				" static line\n" +
				"-      - GF_SMTP_PASSWORD=old_secret\n" +
				"+      - GF_SMTP_PASSWORD=new_secret\n",
			patterns: []maskPattern{
				{prefix: "      - GF_SMTP_PASSWORD=", suffix: ""},
			},
			wantContain: []string{
				"--- f (remote)",
				"+++ f (local)",
				"@@ -1,3 +1,3 @@",
				" static line",
				"-      - GF_SMTP_PASSWORD=" + maskPlaceholder,
				"+      - GF_SMTP_PASSWORD=" + maskPlaceholder,
			},
			wantAbsent: []string{"old_secret", "new_secret"},
		},
		{
			name: "masks context lines too",
			diff: " static\n" +
				" HOST=secret_host\n" +
				"-PORT=old_port\n" +
				"+PORT=new_port\n",
			patterns: []maskPattern{
				{prefix: "HOST=", suffix: ""},
				{prefix: "PORT=", suffix: ""},
			},
			wantContain: []string{
				" static",
				" HOST=" + maskPlaceholder,
				"-PORT=" + maskPlaceholder,
				"+PORT=" + maskPlaceholder,
			},
			wantAbsent: []string{"secret_host", "old_port", "new_port"},
		},
		{
			name: "preserves suffix",
			diff: `-password = """old_pw"""` + "\n" +
				`+password = """new_pw"""` + "\n",
			patterns: []maskPattern{
				{prefix: `password = """`, suffix: `"""`},
			},
			wantContain: []string{
				`-password = """` + maskPlaceholder + `"""`,
				`+password = """` + maskPlaceholder + `"""`,
			},
			wantAbsent: []string{"old_pw", "new_pw"},
		},
		{
			name:        "no patterns returns diff unchanged",
			diff:        "+line1\n-line2\n",
			patterns:    nil,
			wantContain: []string{"+line1", "-line2"},
		},
		{
			name: "non-matching lines are preserved",
			diff: "+unrelated = hello\n+SECRET=hunter2\n",
			patterns: []maskPattern{
				{prefix: "SECRET=", suffix: ""},
			},
			wantContain: []string{"+unrelated = hello"},
			wantAbsent:  []string{"hunter2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := maskDiffWithPatterns(tt.diff, tt.patterns)

			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("result should contain %q, got:\n%s", want, got)
				}
			}

			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("result should NOT contain %q, got:\n%s", absent, got)
				}
			}
		})
	}
}

func TestApplyMaskToLine(t *testing.T) {
	t.Parallel()

	patterns := []maskPattern{
		{prefix: "      - GF_PASS=", suffix: ""},
		{prefix: `pw = """`, suffix: `"""`},
	}

	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "add line masked",
			line: "+      - GF_PASS=new_secret",
			want: "+      - GF_PASS=" + maskPlaceholder,
		},
		{
			name: "remove line masked",
			line: "-      - GF_PASS=old_secret",
			want: "-      - GF_PASS=" + maskPlaceholder,
		},
		{
			name: "context line masked",
			line: "       - GF_PASS=same_secret",
			want: "       - GF_PASS=" + maskPlaceholder,
		},
		{
			name: "suffix preserved",
			line: `+pw = """s3cret"""`,
			want: `+pw = """` + maskPlaceholder + `"""`,
		},
		{
			name: "header preserved",
			line: "--- file (remote)",
			want: "--- file (remote)",
		},
		{
			name: "hunk preserved",
			line: "@@ -1,3 +1,3 @@",
			want: "@@ -1,3 +1,3 @@",
		},
		{
			name: "non-matching line preserved",
			line: "+unrelated = hello",
			want: "+unrelated = hello",
		},
		{
			name: "empty line preserved",
			line: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := applyMaskToLine(tt.line, patterns)
			if got != tt.want {
				t.Errorf("applyMaskToLine(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestLongestCommonPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want string
	}{
		{"DB_HOST=pg.local", "DB_HOST=***", "DB_HOST="},
		{"abc", "abc", "abc"},
		{"abc", "xyz", ""},
		{"", "abc", ""},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			t.Parallel()

			got := longestCommonPrefix(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("longestCommonPrefix(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestLongestCommonSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want string
	}{
		{`s3cret"""`, `***"""`, `"""`},
		{"abc", "abc", "abc"},
		{"abc", "xyz", ""},
		{"", "abc", ""},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			t.Parallel()

			got := longestCommonSuffix(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("longestCommonSuffix(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestBuildMaskedVars(t *testing.T) {
	t.Parallel()

	vars := map[string]any{
		"key1": "value1",
		"key2": 42,
	}

	masked := buildMaskedVars(vars)

	if len(masked) != len(vars) {
		t.Fatalf("len = %d, want %d", len(masked), len(vars))
	}

	for k, v := range masked {
		if v != maskPlaceholder {
			t.Errorf("masked[%q] = %v, want %q", k, v, maskPlaceholder)
		}
	}
}

func TestBuildManifestWithMaskHints(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"compose.yml": "/a/compose.yml",
		".env":        "/a/.env",
	}

	hints := map[string][]MaskHint{
		"compose.yml": {
			{Prefix: "GF_SMTP_PASSWORD=", Suffix: ""},
		},
		"deleted.yml": {
			{Prefix: "SHOULD_NOT_INCLUDE=", Suffix: ""},
		},
	}

	manifest := BuildManifestWithMaskHints(files, hints)

	if len(manifest.ManagedFiles) != 2 {
		t.Fatalf("managed files = %d, want 2", len(manifest.ManagedFiles))
	}

	if len(manifest.MaskHints) != 1 {
		t.Fatalf("mask hints files = %d, want 1", len(manifest.MaskHints))
	}

	composeHints, ok := manifest.MaskHints["compose.yml"]
	if !ok {
		t.Fatal("compose.yml mask hints should exist")
	}

	if len(composeHints) != 1 {
		t.Fatalf("compose.yml hints = %d, want 1", len(composeHints))
	}

	if composeHints[0].Prefix != "GF_SMTP_PASSWORD=" {
		t.Errorf("prefix = %q", composeHints[0].Prefix)
	}
}

func TestMaskDiffWithPatterns_UsesManifestHintsForRemovedSecrets(t *testing.T) {
	t.Parallel()

	diff := `--- compose.yml (remote)
+++ compose.yml (local)
@@ -1,4 +1,3 @@
       - GF_SERVER_ROOT_URL=https://grafana.shiron.dev/
-      - GF_SMTP_PASSWORD=old_secret_value
       - GF_SMTP_HOST=smtp.example.com:587
`

	patterns := mergeMaskPatterns(
		nil,
		patternsFromMaskHints([]MaskHint{
			{Prefix: "      - GF_SMTP_PASSWORD=", Suffix: ""},
		}),
	)

	masked := maskDiffWithPatterns(diff, patterns)

	if strings.Contains(masked, "old_secret_value") {
		t.Fatalf("removed secret should be masked:\n%s", masked)
	}

	if !strings.Contains(masked, "-      - GF_SMTP_PASSWORD="+maskPlaceholder) {
		t.Fatalf("masked removed line not found:\n%s", masked)
	}
}

// ---------------------------------------------------------------------------
// ComposePlan
// ---------------------------------------------------------------------------

func TestComposePlan_HasChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		plan *ComposePlan
		want bool
	}{
		{
			name: "nil plan",
			plan: nil,
			want: false,
		},
		{
			name: "no change",
			plan: &ComposePlan{ActionType: ComposeNoChange},
			want: false,
		},
		{
			name: "start with services",
			plan: &ComposePlan{
				ActionType: ComposeStartServices,
				Services:   []string{"web", "db"},
			},
			want: true,
		},
		{
			name: "stop with services",
			plan: &ComposePlan{
				ActionType: ComposeStopServices,
				Services:   []string{"web"},
			},
			want: true,
		},
		{
			name: "start but empty services",
			plan: &ComposePlan{
				ActionType: ComposeStartServices,
				Services:   nil,
			},
			want: false,
		},
		{
			name: "ignore action keeps no changes",
			plan: &ComposePlan{
				DesiredAction: config.ComposeActionIgnore,
				ActionType:    ComposeNoChange,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.plan.HasChanges(); got != tt.want {
				t.Errorf("HasChanges() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildComposePlan_IgnoreAction(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	composePlan := buildComposePlan(config.ComposeActionIgnore, "/srv/compose/grafana", client)
	if composePlan == nil {
		t.Fatal("compose plan should not be nil")
	}

	if composePlan.DesiredAction != config.ComposeActionIgnore {
		t.Fatalf("DesiredAction = %q, want %q", composePlan.DesiredAction, config.ComposeActionIgnore)
	}

	if composePlan.HasChanges() {
		t.Fatal("ignore action should not produce compose state changes")
	}
}

func TestHasChanges_ComposeOnly(t *testing.T) {
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
							{Exists: true},
						},
						Compose: &ComposePlan{
							ActionType: ComposeStartServices,
							Services:   []string{"web"},
						},
					},
				},
			},
		},
	}

	if !plan.HasChanges() {
		t.Error("HasChanges() should return true when compose has changes")
	}
}

func TestComposeStats(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Projects: []ProjectPlan{
					{
						Compose: &ComposePlan{
							ActionType: ComposeStartServices,
							Services:   []string{"web", "db"},
						},
					},
					{
						Compose: &ComposePlan{
							ActionType: ComposeStopServices,
							Services:   []string{"redis"},
						},
					},
					{
						Compose: nil,
					},
				},
			},
		},
	}

	start, stop := plan.ComposeStats()
	if start != 2 {
		t.Errorf("start = %d, want 2", start)
	}

	if stop != 1 {
		t.Errorf("stop = %d, want 1", stop)
	}
}

func TestParseServiceLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name:   "normal output",
			output: "web\ndb\nredis\n",
			want:   []string{"db", "redis", "web"},
		},
		{
			name:   "empty output",
			output: "",
			want:   nil,
		},
		{
			name:   "whitespace only",
			output: "  \n  \n",
			want:   nil,
		},
		{
			name:   "with trailing whitespace",
			output: "web \n db\n",
			want:   []string{"db", "web"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseServiceLines(tt.output)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d; got = %v", len(got), len(tt.want), got)
			}

			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDiffServices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		all     []string
		running []string
		want    []string
	}{
		{
			name:    "some stopped",
			all:     []string{"db", "redis", "web"},
			running: []string{"web"},
			want:    []string{"db", "redis"},
		},
		{
			name:    "all running",
			all:     []string{"db", "web"},
			running: []string{"db", "web"},
			want:    nil,
		},
		{
			name:    "none running",
			all:     []string{"db", "web"},
			running: nil,
			want:    []string{"db", "web"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := diffServices(tt.all, tt.running)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d; got = %v", len(got), len(tt.want), got)
			}

			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSyncPlan_Print_ComposeServices(t *testing.T) {
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
						ProjectName:   "grafana",
						RemoteDir:     "/opt/compose/grafana",
						ComposeAction: "up",
						Compose: &ComposePlan{
							DesiredAction: "up",
							ActionType:    ComposeStartServices,
							Services:      []string{"grafana", "influxdb"},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
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

	if !strings.Contains(output, "Compose: up") {
		t.Error("output should contain compose action")
	}

	if !strings.Contains(output, "Compose services:") {
		t.Error("output should contain compose services header")
	}

	if !strings.Contains(output, "grafana") {
		t.Error("output should list grafana service")
	}

	if !strings.Contains(output, "influxdb") {
		t.Error("output should list influxdb service")
	}

	if !strings.Contains(output, "(start)") {
		t.Error("output should show (start) for up action")
	}

	if !strings.Contains(output, "service(s) to start") {
		t.Error("summary should mention services to start")
	}
}

func TestBuildPlanWithDeps_ProgressOutput(t *testing.T) {
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
		resolver.EXPECT().Resolve(gomock.Any(), "", hostDir).Return(nil),
		factory.EXPECT().NewClient(gomock.AssignableToTypeOf(config.HostEntry{})).Return(client, nil),
	)
	client.EXPECT().ReadFile("/srv/compose/grafana/.cmt-manifest.json").Return(nil, errors.New("not found"))
	client.EXPECT().ReadFile("/srv/compose/grafana/compose.yml").Return(nil, errors.New("not found"))
	client.EXPECT().RunCommand("/srv/compose/grafana", "docker compose config --services 2>/dev/null").Return("", errors.New("not found"))
	client.EXPECT().RunCommand("/srv/compose/grafana", "docker compose ps --services --filter status=running 2>/dev/null").Return("", errors.New("not found"))
	client.EXPECT().Close().Return(nil)

	var progressBuf bytes.Buffer

	_, err = BuildPlanWithDeps(cfg, nil, nil, PlanDependencies{
		ClientFactory:  factory,
		SSHResolver:    resolver,
		LocalRunner:    mockLocalCommandRunner{run: func(string, []string, string) (string, error) { return "ok", nil }},
		ProgressWriter: &progressBuf,
	})
	if err != nil {
		t.Fatalf("BuildPlanWithDeps: %v", err)
	}

	output := progressBuf.String()

	if !strings.Contains(output, "Planning:") {
		t.Error("progress should contain 'Planning:'")
	}

	if !strings.Contains(output, "1 host(s), 1 project(s)") {
		t.Errorf("progress should show host/project counts, got %q", output)
	}

	if !strings.Contains(output, "Planning host 1/1:") {
		t.Error("progress should contain host progress")
	}

	if !strings.Contains(output, "server1") {
		t.Error("progress should contain host name")
	}

	if !strings.Contains(output, "connecting...") {
		t.Error("progress should show connecting state")
	}

	if !strings.Contains(output, "project 1/1:") {
		t.Errorf("progress should contain project progress, got %q", output)
	}

	if !strings.Contains(output, "grafana") {
		t.Error("progress should contain project name")
	}

	if !strings.Contains(output, "done") {
		t.Error("progress should show done state")
	}

	if !strings.Contains(output, "Plan complete.") {
		t.Error("progress should show plan completion")
	}
}

func TestBuildPlanWithDeps_NoProgressWhenWriterNil(t *testing.T) {
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
		resolver.EXPECT().Resolve(gomock.Any(), "", hostDir).Return(nil),
		factory.EXPECT().NewClient(gomock.AssignableToTypeOf(config.HostEntry{})).Return(client, nil),
	)
	client.EXPECT().ReadFile("/srv/compose/grafana/.cmt-manifest.json").Return(nil, errors.New("not found"))
	client.EXPECT().ReadFile("/srv/compose/grafana/compose.yml").Return(nil, errors.New("not found"))
	client.EXPECT().RunCommand("/srv/compose/grafana", "docker compose config --services 2>/dev/null").Return("", errors.New("not found"))
	client.EXPECT().RunCommand("/srv/compose/grafana", "docker compose ps --services --filter status=running 2>/dev/null").Return("", errors.New("not found"))
	client.EXPECT().Close().Return(nil)

	_, err = BuildPlanWithDeps(cfg, nil, nil, PlanDependencies{
		ClientFactory: factory,
		SSHResolver:   resolver,
		LocalRunner:   mockLocalCommandRunner{run: func(string, []string, string) (string, error) { return "ok", nil }},
	})
	if err != nil {
		t.Fatalf("BuildPlanWithDeps should succeed without progress writer: %v", err)
	}
}
