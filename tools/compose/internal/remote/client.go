package remote

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_remote_interfaces.go -package=remote cmt/internal/remote RemoteClient,ClientFactory,CommandRunner

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"cmt/internal/config"
)

// CommandRunner runs external commands.
type CommandRunner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
}

// ExecCommandRunner runs commands via os/exec.
type ExecCommandRunner struct{}

// CombinedOutput executes a command and returns combined stdout/stderr.
func (ExecCommandRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	return exec.CommandContext(context.Background(), name, args...).CombinedOutput()
}

// RemoteClient is the interface used by sync logic.
type RemoteClient interface {
	ReadFile(remotePath string) ([]byte, error)
	WriteFile(remotePath string, data []byte) error
	MkdirAll(dir string) error
	Remove(remotePath string) error
	Stat(remotePath string) (fs.FileInfo, error)
	ListFilesRecursive(dir string) ([]string, error)
	RunCommand(workdir, command string) (string, error)
	Close() error
}

// ClientFactory creates RemoteClient instances.
type ClientFactory interface {
	NewClient(entry config.HostEntry) (RemoteClient, error)
}

// DefaultClientFactory creates real SSH-backed clients.
type DefaultClientFactory struct {
	Runner CommandRunner
}

// NewClient creates a default client with real command execution.
func (f DefaultClientFactory) NewClient(entry config.HostEntry) (RemoteClient, error) {
	runner := f.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}

	return NewClientWithRunner(entry, runner)
}

var _ RemoteClient = (*Client)(nil)

// Client executes remote operations via external ssh / scp commands.
type Client struct {
	host    config.HostEntry
	sshArgs []string // common args for ssh (without destination)
	scpArgs []string // common args for scp (without source/dest)
	runner  CommandRunner
}

// NewClient builds the common ssh/scp argument lists from the resolved
// HostEntry. No network connection is established at this point.
func NewClient(entry config.HostEntry) (*Client, error) {
	return NewClientWithRunner(entry, ExecCommandRunner{})
}

// NewClientWithRunner builds a client with an injected command runner.
func NewClientWithRunner(entry config.HostEntry, runner CommandRunner) (*Client, error) {
	sshArgs, scpArgs := buildArgs(entry)
	slog.Debug("client created",
		"host", entry.Name,
		"sshArgs", strings.Join(sshArgs, " "),
		"scpArgs", strings.Join(scpArgs, " "),
	)

	return &Client{
		host:    entry,
		sshArgs: sshArgs,
		scpArgs: scpArgs,
		runner:  runner,
	}, nil
}

// Close is a no-op (each command opens and closes its own connection).
func (c *Client) Close() error { return nil }

// ---------------------------------------------------------------------------
// Remote operations
// ---------------------------------------------------------------------------

// ReadFile returns the contents of a remote file.
func (c *Client) ReadFile(remotePath string) ([]byte, error) {
	out, err := c.runSSH("cat " + shellQuote(remotePath))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", remotePath, err)
	}

	return out, nil
}

// WriteFile writes data to a remote file, creating parent directories.
func (c *Client) WriteFile(remotePath string, data []byte) error {
	dir := path.Dir(remotePath)

	err := c.MkdirAll(dir)
	if err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	// Write to a local temp file, then scp it to the remote host.
	tmp, err := os.CreateTemp("", "cmt-upload-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	tmpPath := tmp.Name()

	defer func() {
		_ = os.Remove(tmpPath)
	}()

	_, err = tmp.Write(data)
	if err != nil {
		_ = tmp.Close()

		return fmt.Errorf("write temp file: %w", err)
	}

	closeErr := tmp.Close()
	if closeErr != nil {
		return fmt.Errorf("close temp file: %w", closeErr)
	}

	err = c.runSCP(tmpPath, remotePath)
	if err != nil {
		return fmt.Errorf("scp to %s: %w", remotePath, err)
	}

	return nil
}

// MkdirAll creates remote directories recursively.
func (c *Client) MkdirAll(dir string) error {
	_, err := c.runSSH("mkdir -p " + shellQuote(dir))

	return err
}

// Remove deletes a remote file.
func (c *Client) Remove(remotePath string) error {
	_, err := c.runSSH("rm -f " + shellQuote(remotePath))

	return err
}

// Stat checks whether a remote path exists.
// It returns a minimal fs.FileInfo on success; callers should only
// rely on the error (nil = exists) since the info is a placeholder.
func (c *Client) Stat(remotePath string) (fs.FileInfo, error) {
	_, err := c.runSSH("test -e " + shellQuote(remotePath))
	if err != nil {
		return nil, fmt.Errorf("stat %s: path does not exist", remotePath)
	}

	return minimalFileInfo{name: path.Base(remotePath)}, nil
}

// ListFilesRecursive returns relative paths of all files under dir.
func (c *Client) ListFilesRecursive(dir string) ([]string, error) {
	out, err := c.runSSH(fmt.Sprintf(
		"find %s -type f 2>/dev/null || true", shellQuote(dir),
	))
	if err != nil {
		return nil, err
	}

	var files []string

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}

		rel := strings.TrimPrefix(line, dir+"/")
		if rel != line {
			files = append(files, rel)
		}
	}

	return files, nil
}

// RunCommand executes a command on the remote host.
// If workdir is non-empty, the command is prefixed with cd.
func (c *Client) RunCommand(workdir, command string) (string, error) {
	cmd := command
	if workdir != "" {
		cmd = fmt.Sprintf("cd %s && %s", shellQuote(workdir), command)
	}

	out, err := c.runSSH(cmd)

	return string(out), err
}

// ---------------------------------------------------------------------------
// SSH / SCP execution helpers
// ---------------------------------------------------------------------------

// runSSH executes `ssh <common-args> <host> -- <remoteCmd>` and returns
// combined stdout+stderr output.
func (c *Client) runSSH(remoteCmd string) ([]byte, error) {
	const sshArgsPadding = 3

	args := make([]string, 0, len(c.sshArgs)+sshArgsPadding)
	args = append(args, c.sshArgs...)
	args = append(args, c.host.Host, "--", remoteCmd)

	slog.Debug("running ssh", "command", "ssh "+strings.Join(args, " "))

	out, err := c.runner.CombinedOutput("ssh", args...)
	if err != nil {
		return out, fmt.Errorf("ssh %s: %w\n%s", c.host.Name, err, out)
	}

	return out, nil
}

// runSCP executes `scp <common-args> <localPath> <user@host:remotePath>`.
func (c *Client) runSCP(localPath, remotePath string) error {
	dest := fmt.Sprintf("%s@%s:%s", c.host.User, c.host.Host, remotePath)

	const scpArgsPadding = 2

	args := make([]string, 0, len(c.scpArgs)+scpArgsPadding)
	args = append(args, c.scpArgs...)
	args = append(args, localPath, dest)

	slog.Debug("running scp", "command", "scp "+strings.Join(args, " "))

	out, err := c.runner.CombinedOutput("scp", args...)
	if err != nil {
		return fmt.Errorf("scp to %s: %w\n%s", dest, err, out)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Argument builders
// ---------------------------------------------------------------------------

// buildArgs constructs the shared argument slices for ssh and scp from the
// resolved HostEntry.
func buildArgs(entry config.HostEntry) ([]string, []string) {
	var (
		sshArgs []string
		scpArgs []string
	)

	commonOpts := buildCommonOptions(entry)

	// ssh uses -p for port, -l for user.
	sshArgs = append(sshArgs, commonOpts...)
	if entry.Port != 0 && entry.Port != 22 {
		sshArgs = append(sshArgs, "-p", strconv.Itoa(entry.Port))
	}

	if entry.User != "" {
		sshArgs = append(sshArgs, "-l", entry.User)
	}

	// scp uses -P (uppercase) for port; user is encoded in the destination.
	scpArgs = append(scpArgs, commonOpts...)
	if entry.Port != 0 && entry.Port != 22 {
		scpArgs = append(scpArgs, "-P", strconv.Itoa(entry.Port))
	}

	return sshArgs, scpArgs
}

func buildCommonOptions(entry config.HostEntry) []string {
	commonOpts := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "BatchMode=yes",
	}

	if entry.ProxyCommand != "" {
		commonOpts = append(commonOpts, "-o", "ProxyCommand="+entry.ProxyCommand)
	}

	if entry.IdentityAgent != "" && entry.IdentityAgent != "none" {
		commonOpts = append(commonOpts, "-o", "IdentityAgent="+entry.IdentityAgent)
	}

	for _, keyPath := range entry.IdentityFiles {
		commonOpts = append(commonOpts, "-i", keyPath)
	}

	if entry.SSHKeyPath != "" && len(entry.IdentityFiles) == 0 {
		commonOpts = append(commonOpts, "-i", entry.SSHKeyPath)
	}

	return commonOpts
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// minimalFileInfo satisfies fs.FileInfo for callers that only check the error.
type minimalFileInfo struct{ name string }

func (m minimalFileInfo) Name() string       { return m.name }
func (m minimalFileInfo) Size() int64        { return 0 }
func (m minimalFileInfo) Mode() fs.FileMode  { return 0 }
func (m minimalFileInfo) ModTime() time.Time { return time.Time{} }
func (m minimalFileInfo) IsDir() bool        { return false }
func (m minimalFileInfo) Sys() any           { return nil }
