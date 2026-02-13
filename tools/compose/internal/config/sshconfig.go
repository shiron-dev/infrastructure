package config

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_ssh_interfaces.go -package=config cmt/internal/config SSHConfigRunner,SSHConfigResolver

import (
	"bufio"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// SSHConfigRunner runs external commands for SSH config resolution.
type SSHConfigRunner interface {
	Output(name string, args ...string) ([]byte, error)
}

// ExecSSHConfigRunner resolves SSH config via os/exec.
type ExecSSHConfigRunner struct{}

// Output executes a command and returns stdout.
func (ExecSSHConfigRunner) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// SSHConfigResolver resolves and applies SSH config to a host entry.
type SSHConfigResolver interface {
	Resolve(entry *HostEntry, sshConfigPath, hostDir string) error
}

// DefaultSSHConfigResolver resolves using ssh -G with an injected runner.
type DefaultSSHConfigResolver struct {
	Runner SSHConfigRunner
}

// Resolve resolves SSH config for entry.
func (r DefaultSSHConfigResolver) Resolve(entry *HostEntry, sshConfigPath, hostDir string) error {
	runner := r.Runner
	if runner == nil {
		runner = ExecSSHConfigRunner{}
	}

	return ResolveSSHConfigWithRunner(entry, sshConfigPath, hostDir, runner)
}

// ResolveSSHConfig runs `ssh -G` to resolve SSH configuration for the given
// host entry and overrides its fields with the resolved values.
// sshConfigPath is passed via -F when non-empty; relative paths are resolved
// against hostDir.
func ResolveSSHConfig(entry *HostEntry, sshConfigPath, hostDir string) error {
	return ResolveSSHConfigWithRunner(entry, sshConfigPath, hostDir, ExecSSHConfigRunner{})
}

// ResolveSSHConfigWithRunner is ResolveSSHConfig with an injected command runner.
func ResolveSSHConfigWithRunner(entry *HostEntry, sshConfigPath, hostDir string, runner SSHConfigRunner) error {
	originalHost := entry.Host

	args := []string{"-G"}

	if sshConfigPath != "" {
		if !filepath.IsAbs(sshConfigPath) {
			sshConfigPath = filepath.Join(hostDir, sshConfigPath)
		}

		args = append(args, "-F", sshConfigPath)
	}

	if entry.User != "" {
		args = append(args, "-l", entry.User)
	}

	if entry.Port != 0 {
		args = append(args, "-p", strconv.Itoa(entry.Port))
	}

	args = append(args, entry.Host)

	slog.Debug("running ssh -G", "command", "ssh "+strings.Join(args, " "), "host", entry.Name)

	out, err := runner.Output("ssh", args...)
	if err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			return fmt.Errorf("ssh -G %s: %w\nstderr: %s", entry.Host, err, exitErr.Stderr)
		}

		return fmt.Errorf("ssh -G %s: %w", entry.Host, err)
	}

	single, multi := parseSSHGOutput(string(out))

	// Hostname is always taken from ssh -G (resolves aliases).
	if v, ok := single["hostname"]; ok {
		entry.Host = v
	}
	// User and port from YAML take precedence; ssh -G fills them only
	// when the YAML value is at its zero value (empty / 0).
	if entry.User == "" {
		if v, ok := single["user"]; ok {
			entry.User = v
		}
	}

	if entry.Port == 0 {
		if v, ok := single["port"]; ok {
			if port, err := strconv.Atoi(v); err == nil && port > 0 {
				entry.Port = port
			}
		}
	}
	// Final default if still unset.
	if entry.Port == 0 {
		entry.Port = 22
	}

	// ProxyCommand — expand placeholders eagerly.
	if v, ok := single["proxycommand"]; ok && v != "none" {
		entry.ProxyCommand = expandProxyPlaceholders(
			v, entry.Host, entry.Port, entry.User, originalHost,
		)
	}

	// Identity files (may appear multiple times in ssh -G output).
	// Expand SSH placeholders that ssh -G may leave unexpanded.
	if files, ok := multi["identityfile"]; ok {
		expanded := make([]string, len(files))
		for i, f := range files {
			expanded[i] = expandProxyPlaceholders(
				f, entry.Host, entry.Port, entry.User, originalHost,
			)
		}

		entry.IdentityFiles = expanded
	}

	// Identity agent socket.
	if v, ok := single["identityagent"]; ok {
		entry.IdentityAgent = v
	}

	slog.Debug("ssh -G resolved",
		"host", entry.Name,
		"hostname", entry.Host,
		"user", entry.User,
		"port", entry.Port,
		"proxycommand", entry.ProxyCommand,
		"identityfiles", entry.IdentityFiles,
		"identityagent", entry.IdentityAgent,
	)

	return nil
}

// parseSSHGOutput parses `ssh -G` key-value output into single-valued and
// multi-valued maps. Keys are lowercased.
func parseSSHGOutput(output string) (single map[string]string, multi map[string][]string) {
	single = make(map[string]string)
	multi = make(map[string][]string)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), " ")
		if !ok {
			continue
		}

		key = strings.ToLower(key)
		single[key] = value
		multi[key] = append(multi[key], value)
	}

	return
}

// expandProxyPlaceholders expands SSH proxy command tokens:
//
//	%h → resolved hostname
//	%p → resolved port
//	%r → remote username
//	%n → original hostname (before resolution)
//	%% → literal %
func expandProxyPlaceholders(cmd, hostname string, port int, user, originalHost string) string {
	cmd = strings.ReplaceAll(cmd, "%%", "\x00")
	cmd = strings.ReplaceAll(cmd, "%h", hostname)
	cmd = strings.ReplaceAll(cmd, "%p", strconv.Itoa(port))
	cmd = strings.ReplaceAll(cmd, "%r", user)
	cmd = strings.ReplaceAll(cmd, "%n", originalHost)
	cmd = strings.ReplaceAll(cmd, "\x00", "%")

	return cmd
}
