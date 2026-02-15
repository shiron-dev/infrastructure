package config

import (
	"path/filepath"
	"testing"

	"go.uber.org/mock/gomock"
)

// ---------------------------------------------------------------------------
// parseSSHGOutput
// ---------------------------------------------------------------------------

func TestParseSSHGOutput(t *testing.T) {
	t.Parallel()

	assertSingle := func(t *testing.T, single map[string]string, key, want string) {
		t.Helper()

		if single[key] != want {
			t.Errorf("%s = %q, want %q", key, single[key], want)
		}
	}

	assertMulti := func(t *testing.T, multi map[string][]string, key string, want []string) {
		t.Helper()

		got := multi[key]
		if len(got) != len(want) {
			t.Fatalf("%s length = %d, want %d", key, len(got), len(want))
		}

		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s[%d] = %q, want %q", key, i, got[i], want[i])
			}
		}
	}

	testCases := []struct {
		name     string
		input    string
		validate func(*testing.T, map[string]string, map[string][]string)
	}{
		{
			name:  "basic key-value pairs",
			input: "hostname 192.168.1.1\nuser deploy\nport 2222\n",
			validate: func(t *testing.T, single map[string]string, multi map[string][]string) {
				t.Helper()
				assertSingle(t, single, "hostname", "192.168.1.1")
				assertSingle(t, single, "user", "deploy")
				assertSingle(t, single, "port", "2222")
				assertMulti(t, multi, "hostname", []string{"192.168.1.1"})
			},
		},
		{
			name:  "keys are lowercased",
			input: "HostName example.com\nUser admin\nPort 22\n",
			validate: func(t *testing.T, single map[string]string, _ map[string][]string) {
				t.Helper()
				assertSingle(t, single, "hostname", "example.com")
				assertSingle(t, single, "user", "admin")
				assertSingle(t, single, "port", "22")
			},
		},
		{
			name:  "multi-value keys",
			input: "identityfile ~/.ssh/id_rsa\nidentityfile ~/.ssh/id_ed25519\nhostname host1\n",
			validate: func(t *testing.T, single map[string]string, multi map[string][]string) {
				t.Helper()
				assertSingle(t, single, "identityfile", "~/.ssh/id_ed25519")
				assertMulti(t, multi, "identityfile", []string{"~/.ssh/id_rsa", "~/.ssh/id_ed25519"})
			},
		},
		{
			name:  "invalid lines are skipped",
			input: "hostname example.com\nno-space-line\n\nport 22\n",
			validate: func(t *testing.T, single map[string]string, _ map[string][]string) {
				t.Helper()
				assertSingle(t, single, "hostname", "example.com")
				assertSingle(t, single, "port", "22")

				if _, ok := single["no-space-line"]; ok {
					t.Error("no-space-line should be skipped")
				}
			},
		},
		{
			name:  "empty input",
			input: "",
			validate: func(t *testing.T, single map[string]string, multi map[string][]string) {
				t.Helper()

				if len(single) != 0 {
					t.Errorf("expected empty single, got %v", single)
				}

				if len(multi) != 0 {
					t.Errorf("expected empty multi, got %v", multi)
				}
			},
		},
		{
			name:  "value with spaces",
			input: "proxycommand ssh -W %h:%p bastion\n",
			validate: func(t *testing.T, single map[string]string, _ map[string][]string) {
				t.Helper()
				assertSingle(t, single, "proxycommand", "ssh -W %h:%p bastion")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			single, multi := parseSSHGOutput(tc.input)
			tc.validate(t, single, multi)
		})
	}
}

// ---------------------------------------------------------------------------
// expandProxyPlaceholders
// ---------------------------------------------------------------------------

func TestExpandProxyPlaceholders(t *testing.T) {
	t.Parallel()

	t.Run("all placeholders", func(t *testing.T) {
		t.Parallel()

		result := expandProxyPlaceholders(
			"ssh -W %h:%p -l %r %n",
			"192.168.1.1", 2222, "deploy", "myhost",
		)

		expected := "ssh -W 192.168.1.1:2222 -l deploy myhost"

		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
	})

	t.Run("percent-percent escaping", func(t *testing.T) {
		t.Parallel()

		result := expandProxyPlaceholders(
			"echo %%h is %h",
			"resolved.host", 22, "user", "orig",
		)

		expected := "echo %h is resolved.host"

		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
	})

	t.Run("no placeholders", func(t *testing.T) {
		t.Parallel()

		result := expandProxyPlaceholders(
			"nc bastion 22",
			"host", 22, "user", "orig",
		)
		if result != "nc bastion 22" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("multiple percent-percent", func(t *testing.T) {
		t.Parallel()

		result := expandProxyPlaceholders(
			"a%%b%%c",
			"h", 1, "u", "o",
		)
		if result != "a%b%c" {
			t.Errorf("got %q, want %q", result, "a%b%c")
		}
	})

	t.Run("repeated placeholders", func(t *testing.T) {
		t.Parallel()

		result := expandProxyPlaceholders(
			"%h-%h-%p-%p",
			"host", 80, "user", "orig",
		)
		if result != "host-host-80-80" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("identity file path expansion", func(t *testing.T) {
		t.Parallel()

		// Identity files also use expandProxyPlaceholders.
		result := expandProxyPlaceholders(
			"/home/%r/.ssh/id_%n",
			"10.0.0.1", 22, "admin", "myserver",
		)

		expected := "/home/admin/.ssh/id_myserver"

		if result != expected {
			t.Errorf("got %q, want %q", result, expected)
		}
	})
}

func TestResolveSSHConfigWithRunner(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	runner := NewMockSSHConfigRunner(ctrl)

	entry := &HostEntry{
		Name: "server1",
		Host: "server1-alias",
		User: "",
		Port: 0,
	}

	hostDir := "/tmp/base/hosts/server1"
	sshConfigPath := "ssh_config"

	resolved := "" +
		"hostname 10.0.0.10\n" +
		"user deploy\n" +
		"port 2222\n" +
		"proxycommand ssh -W %h:%p bastion\n" +
		"identityfile /home/%r/.ssh/id_%n\n" +
		"identityagent /tmp/agent.sock\n"

	runner.EXPECT().
		Output(
			"ssh",
			"-G",
			"-F",
			filepath.Join(hostDir, sshConfigPath),
			"server1-alias",
		).
		Return([]byte(resolved), nil)

	err := ResolveSSHConfigWithRunner(entry, sshConfigPath, hostDir, runner)
	if err != nil {
		t.Fatalf("ResolveSSHConfigWithRunner: %v", err)
	}

	if entry.Host != "10.0.0.10" {
		t.Errorf("Host = %q, want 10.0.0.10", entry.Host)
	}

	if entry.User != "deploy" {
		t.Errorf("User = %q, want deploy", entry.User)
	}

	if entry.Port != 2222 {
		t.Errorf("Port = %d, want 2222", entry.Port)
	}

	if entry.ProxyCommand != "ssh -W 10.0.0.10:2222 bastion" {
		t.Errorf("ProxyCommand = %q", entry.ProxyCommand)
	}

	if len(entry.IdentityFiles) != 1 || entry.IdentityFiles[0] != "/home/deploy/.ssh/id_server1-alias" {
		t.Errorf("IdentityFiles = %v", entry.IdentityFiles)
	}

	if entry.IdentityAgent != "/tmp/agent.sock" {
		t.Errorf("IdentityAgent = %q", entry.IdentityAgent)
	}
}
