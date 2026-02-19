package syncer

import (
	"bytes"
	"strings"
	"testing"

	"cmt/internal/config"
	"cmt/internal/remote"

	"go.uber.org/mock/gomock"
)

func TestApplyWithDeps_Cancelled(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Files: []FilePlan{
							{RelativePath: "compose.yml", Action: ActionAdd, LocalPath: "/tmp/compose.yml", LocalData: []byte("x")},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)

	var out bytes.Buffer

	err := ApplyWithDeps(&config.CmtConfig{}, plan, false, false, &out, ApplyDependencies{
		ClientFactory: factory,
		Input:         strings.NewReader("n\n"),
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply cancelled.") {
		t.Fatalf("expected cancel output, got %q", out.String())
	}
}

func TestApplyWithDeps_UsesInjectedClientFactory(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName:     "grafana",
						RemoteDir:       "/srv/grafana",
						PostSyncCommand: "echo done",
						Dirs: []DirPlan{
							{RelativePath: "data", RemotePath: "/srv/grafana/data", Exists: false},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionAdd,
								LocalData:    []byte("services: {}"),
							},
							{
								RelativePath: "old.txt",
								RemotePath:   "/srv/grafana/old.txt",
								Action:       ActionDelete,
							},
							{
								RelativePath: ".env",
								LocalPath:    "/tmp/.env",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().MkdirAll("/srv/grafana/data").Return(nil),
		client.EXPECT().WriteFile("/srv/grafana/compose.yml", []byte("services: {}")).Return(nil),
		client.EXPECT().Remove("/srv/grafana/old.txt").Return(nil),
		client.EXPECT().WriteFile("/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().RunCommand("/srv/grafana", "echo done").Return("ok", nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(&config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}

func TestApplyWithDeps_BeforePromptHook_Rejected(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Files: []FilePlan{
							{RelativePath: "compose.yml", Action: ActionAdd, LocalPath: "/tmp/compose.yml", LocalData: []byte("x")},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)

	cfg := &config.CmtConfig{
		BeforeApplyHooks: &config.BeforeApplyHooks{
			BeforePrompt: &config.HookCommand{Command: "reject"},
		},
	}

	mockRunner := func(_ string, _ []byte) (int, string, error) {
		return 1, "rejected by policy", nil
	}

	var out bytes.Buffer

	err := ApplyWithDeps(cfg, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
		HookRunner:    mockRunner,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Apply cancelled by hook.") {
		t.Fatalf("expected hook cancel output, got %q", output)
	}
}

func TestApplyWithDeps_AfterPromptHook_Rejected(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Files: []FilePlan{
							{RelativePath: "compose.yml", Action: ActionAdd, LocalPath: "/tmp/compose.yml", LocalData: []byte("x")},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)

	cfg := &config.CmtConfig{
		BeforeApplyHooks: &config.BeforeApplyHooks{
			AfterPrompt: &config.HookCommand{Command: "reject"},
		},
	}

	mockRunner := func(_ string, _ []byte) (int, string, error) {
		return 1, "", nil
	}

	var out bytes.Buffer

	err := ApplyWithDeps(cfg, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
		HookRunner:    mockRunner,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Apply cancelled by hook.") {
		t.Fatalf("expected hook cancel output, got %q", output)
	}
}

func TestApplyWithDeps_BeforePromptHook_ErrorExitCode(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Files: []FilePlan{
							{RelativePath: "compose.yml", Action: ActionAdd, LocalPath: "/tmp/compose.yml", LocalData: []byte("x")},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)

	cfg := &config.CmtConfig{
		BeforeApplyHooks: &config.BeforeApplyHooks{
			BeforePrompt: &config.HookCommand{Command: "fail"},
		},
	}

	mockRunner := func(_ string, _ []byte) (int, string, error) {
		return 2, "unexpected error", nil
	}

	var out bytes.Buffer

	err := ApplyWithDeps(cfg, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
		HookRunner:    mockRunner,
	})
	if err == nil {
		t.Fatal("expected error from hook exit code 2")
	}

	if !strings.Contains(err.Error(), "beforePrompt hook failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyWithDeps_BothHooks_Pass(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionAdd,
								LocalData:    []byte("services: {}"),
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().WriteFile("/srv/grafana/compose.yml", []byte("services: {}")).Return(nil),
		client.EXPECT().WriteFile("/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().Close().Return(nil),
	)

	cfg := &config.CmtConfig{
		BeforeApplyHooks: &config.BeforeApplyHooks{
			BeforePrompt: &config.HookCommand{Command: "check-policy"},
			AfterPrompt:  &config.HookCommand{Command: "final-gate"},
		},
	}

	hookCalls := 0
	mockRunner := func(_ string, _ []byte) (int, string, error) {
		hookCalls++

		return 0, "", nil
	}

	var out bytes.Buffer

	err := ApplyWithDeps(cfg, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
		HookRunner:    mockRunner,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if hookCalls != 2 {
		t.Fatalf("expected 2 hook calls, got %d", hookCalls)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}

func TestApplyWithDeps_RefreshManifestOnNoop(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								Action:       ActionUnchanged,
								MaskHints: []MaskHint{
									{Prefix: "      - GF_SMTP_PASSWORD=", Suffix: ""},
								},
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().
			WriteFile("/srv/grafana/.cmt-manifest.json", gomock.Any()).
			Return(nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(&config.CmtConfig{}, plan, true, true, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "No changes to apply.") {
		t.Fatalf("expected noop output, got %q", output)
	}

	if !strings.Contains(output, "Manifest refreshed.") {
		t.Fatalf("expected manifest refreshed output, got %q", output)
	}
}

func TestApplyWithDeps_ComposeUp(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName:   "grafana",
						RemoteDir:     "/srv/grafana",
						ComposeAction: "up",
						Compose: &ComposePlan{
							DesiredAction: "up",
							ActionType:    ComposeStartServices,
							Services:      []string{"grafana", "influxdb"},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().WriteFile("/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().RunCommand("/srv/grafana", "docker compose up -d").Return("ok", nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(&config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Apply complete!") {
		t.Fatalf("expected complete output, got %q", output)
	}

	if !strings.Contains(output, "compose") {
		t.Fatalf("expected compose output, got %q", output)
	}
}

func TestApplyWithDeps_ComposeDown(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName:   "grafana",
						RemoteDir:     "/srv/grafana",
						ComposeAction: "down",
						Compose: &ComposePlan{
							DesiredAction: "down",
							ActionType:    ComposeStopServices,
							Services:      []string{"grafana"},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().WriteFile("/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().RunCommand("/srv/grafana", "docker compose down").Return("", nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(&config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Apply complete!") {
		t.Fatalf("expected complete output, got %q", output)
	}
}

func TestProjectHasChanges_ComposeOnly(t *testing.T) {
	t.Parallel()

	pp := ProjectPlan{
		Files: []FilePlan{
			{Action: ActionUnchanged},
		},
		Compose: &ComposePlan{
			ActionType: ComposeStartServices,
			Services:   []string{"web"},
		},
	}

	if !projectHasChanges(pp) {
		t.Error("projectHasChanges should return true when compose has changes")
	}
}

func TestProjectHasChanges_NoCompose(t *testing.T) {
	t.Parallel()

	pp := ProjectPlan{
		Files: []FilePlan{
			{Action: ActionUnchanged},
		},
		Compose: nil,
	}

	if projectHasChanges(pp) {
		t.Error("projectHasChanges should return false when no changes")
	}
}
