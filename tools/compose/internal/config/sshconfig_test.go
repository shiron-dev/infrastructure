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

	t.Run("basic key-value pairs", func(t *testing.T) {
		t.Parallel()

		input := "hostname 192.168.1.1\nuser deploy\nport 2222\n"
		single, multi := parseSSHGOutput(input)

		if single["hostname"] != "192.168.1.1" {
			t.Errorf("hostname = %q, want 192.168.1.1", single["hostname"])
		}

		if single["user"] != "deploy" {
			t.Errorf("user = %q, want deploy", single["user"])
		}

		if single["port"] != "2222" {
			t.Errorf("port = %q, want 2222", single["port"])
		}

		// multi should also have them
		if len(multi["hostname"]) != 1 || multi["hostname"][0] != "192.168.1.1" {
			t.Errorf("multi hostname = %v", multi["hostname"])
		}
	})

	t.Run("keys are lowercased", func(t *testing.T) {
		t.Parallel()

		input := "HostName example.com\nUser admin\nPort 22\n"
		single, _ := parseSSHGOutput(input)

		if single["hostname"] != "example.com" {
			t.Errorf("hostname = %q, want example.com", single["hostname"])
		}

		if single["user"] != "admin" {
			t.Errorf("user = %q, want admin", single["user"])
		}

		if single["port"] != "22" {
			t.Errorf("port = %q, want 22", single["port"])
		}
	})

	t.Run("multi-value keys", func(t *testing.T) {
		t.Parallel()

		input := "identityfile ~/.ssh/id_rsa\nidentityfile ~/.ssh/id_ed25519\nhostname host1\n"
		single, multi := parseSSHGOutput(input)

		// single keeps the last value
		if single["identityfile"] != "~/.ssh/id_ed25519" {
			t.Errorf("single identityfile = %q, want ~/.ssh/id_ed25519", single["identityfile"])
		}

		// multi keeps all values
		if len(multi["identityfile"]) != 2 {
			t.Fatalf("expected 2 identity files, got %d", len(multi["identityfile"]))
		}

		if multi["identityfile"][0] != "~/.ssh/id_rsa" {
			t.Errorf("multi identityfile[0] = %q", multi["identityfile"][0])
		}

		if multi["identityfile"][1] != "~/.ssh/id_ed25519" {
			t.Errorf("multi identityfile[1] = %q", multi["identityfile"][1])
		}
	})

	t.Run("invalid lines are skipped", func(t *testing.T) {
		t.Parallel()

		input := "hostname example.com\nno-space-line\n\nport 22\n"
		single, _ := parseSSHGOutput(input)

		if single["hostname"] != "example.com" {
			t.Errorf("hostname = %q", single["hostname"])
		}

		if single["port"] != "22" {
			t.Errorf("port = %q", single["port"])
		}

		if _, ok := single["no-space-line"]; ok {
			t.Error("no-space-line should be skipped")
		}
	})

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()

		single, multi := parseSSHGOutput("")
		if len(single) != 0 {
			t.Errorf("expected empty single, got %v", single)
		}

		if len(multi) != 0 {
			t.Errorf("expected empty multi, got %v", multi)
		}
	})

	t.Run("value with spaces", func(t *testing.T) {
		t.Parallel()

		input := "proxycommand ssh -W %h:%p bastion\n"
		single, _ := parseSSHGOutput(input)

		if single["proxycommand"] != "ssh -W %h:%p bastion" {
			t.Errorf("proxycommand = %q, want %q", single["proxycommand"], "ssh -W %h:%p bastion")
		}
	})
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
