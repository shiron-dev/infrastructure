package syncer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"cmt/internal/config"
	"cmt/internal/remote"
)

// ApplyDependencies holds injectable dependencies for Apply.
type ApplyDependencies struct {
	ClientFactory remote.ClientFactory
	Input         io.Reader
}

// Apply executes a SyncPlan: uploads / deletes files, updates manifests,
// and runs post-sync commands.
//
// If autoApprove is false, the plan is printed and the user is prompted
// for confirmation before any changes are made.
func Apply(cfg *config.CmtConfig, plan *SyncPlan, autoApprove bool, w io.Writer) error {
	return ApplyWithDeps(cfg, plan, autoApprove, w, ApplyDependencies{})
}

// ApplyWithDeps executes Apply with injected dependencies.
func ApplyWithDeps(cfg *config.CmtConfig, plan *SyncPlan, autoApprove bool, w io.Writer, deps ApplyDependencies) error {
	clientFactory := deps.ClientFactory
	if clientFactory == nil {
		clientFactory = remote.DefaultClientFactory{}
	}

	input := deps.Input
	if input == nil {
		input = os.Stdin
	}

	if !plan.HasChanges() {
		fmt.Fprintln(w, "No changes to apply.")

		return nil
	}

	// Show the plan first.
	plan.Print(w)

	// Confirm unless --auto-approve.
	if !autoApprove {
		fmt.Fprint(w, "\nApply these changes? (y/N): ")

		reader := bufio.NewReader(input)
		ans, _ := reader.ReadString('\n')

		ans = strings.TrimSpace(strings.ToLower(ans))

		if ans != "y" && ans != "yes" {
			fmt.Fprintln(w, "Apply cancelled.")

			return nil
		}
	}

	fmt.Fprintln(w)

	for _, hp := range plan.HostPlans {
		fmt.Fprintf(w, "Applying to %s...\n", hp.Host.Name)

		client, err := clientFactory.NewClient(hp.Host)
		if err != nil {
			return fmt.Errorf("connecting to %s: %w", hp.Host.Name, err)
		}

		if err := applyHostPlan(cfg, hp, client, w); err != nil {
			client.Close()

			return err
		}

		client.Close()
	}

	_, _, add, mod, del, _ := plan.Stats()
	fmt.Fprintf(w, "\nApply complete! %d file(s) synced (%d added, %d modified, %d deleted)\n",
		add+mod+del, add, mod, del)

	return nil
}

func applyHostPlan(cfg *config.CmtConfig, hp HostPlan, client remote.RemoteClient, w io.Writer) error {
	for _, pp := range hp.Projects {
		hasChanges := false

		for _, fp := range pp.Files {
			if fp.Action != ActionUnchanged {
				hasChanges = true

				break
			}
		}

		if !hasChanges {
			fmt.Fprintf(w, "  %s: no changes\n", pp.ProjectName)

			continue
		}

		fmt.Fprintf(w, "  %s:\n", pp.ProjectName)

		// Create pre-configured directories.
		for _, dp := range pp.Dirs {
			if dp.Exists {
				continue
			}

			fmt.Fprintf(w, "    creating dir %s/... ", dp.RelativePath)
			err := client.MkdirAll(dp.RemotePath)

			if err != nil {
				fmt.Fprintln(w, "FAILED")

				return fmt.Errorf("creating directory %s: %w", dp.RemotePath, err)
			}

			fmt.Fprintln(w, "done")
		}

		// Collect managed files for manifest.
		localFiles := make(map[string]string)

		for _, fp := range pp.Files {
			switch fp.Action {
			case ActionAdd, ActionModify:
				fmt.Fprintf(w, "    uploading %s... ", fp.RelativePath)
				err := client.WriteFile(fp.RemotePath, fp.LocalData)

				if err != nil {
					fmt.Fprintln(w, "FAILED")

					return fmt.Errorf("writing %s: %w", fp.RemotePath, err)
				}

				fmt.Fprintln(w, "done")

				localFiles[fp.RelativePath] = fp.LocalPath

			case ActionDelete:
				fmt.Fprintf(w, "    deleting %s... ", fp.RelativePath)
				err := client.Remove(fp.RemotePath)

				if err != nil {
					fmt.Fprintln(w, "FAILED")

					return fmt.Errorf("deleting %s: %w", fp.RemotePath, err)
				}

				fmt.Fprintln(w, "done")

			case ActionUnchanged:
				localFiles[fp.RelativePath] = fp.LocalPath
			}
		}

		// Write updated manifest.
		manifest := BuildManifest(localFiles)
		manifestData, _ := json.MarshalIndent(manifest, "", "  ")

		manifestPath := path.Join(pp.RemoteDir, manifestFile)
		err := client.WriteFile(manifestPath, manifestData)

		if err != nil {
			return fmt.Errorf("writing manifest: %w", err)
		}

		// Run post-sync command.
		if pp.PostSyncCommand != "" {
			fmt.Fprintf(w, "    running post-sync command... ")

			out, err := client.RunCommand(pp.RemoteDir, pp.PostSyncCommand)
			if err != nil {
				fmt.Fprintln(w, "FAILED")

				if out != "" {
					fmt.Fprintf(w, "    output: %s\n", out)
				}

				return fmt.Errorf("post-sync command on %s/%s: %w", hp.Host.Name, pp.ProjectName, err)
			}

			fmt.Fprintln(w, "done")

			if out != "" {
				for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
					fmt.Fprintf(w, "      %s\n", line)
				}
			}
		}
	}

	return nil
}
