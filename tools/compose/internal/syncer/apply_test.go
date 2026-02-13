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

	err := ApplyWithDeps(&config.CmtConfig{}, plan, false, &out, ApplyDependencies{
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

	err := ApplyWithDeps(&config.CmtConfig{}, plan, true, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}
