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
	var dependencies ApplyDependencies

	return ApplyWithDeps(cfg, plan, autoApprove, w, dependencies)
}

// ApplyWithDeps executes Apply with injected dependencies.
func ApplyWithDeps(cfg *config.CmtConfig, plan *SyncPlan, autoApprove bool, writer io.Writer, deps ApplyDependencies) error {
	style := newOutputStyle(writer)

	clientFactory := deps.ClientFactory
	if clientFactory == nil {
		defaultFactory := new(remote.DefaultClientFactory)
		defaultFactory.Runner = nil
		clientFactory = *defaultFactory
	}

	input := deps.Input
	if input == nil {
		input = os.Stdin
	}

	if !plan.HasChanges() {
		_, _ = fmt.Fprintln(writer, style.muted("No changes to apply."))

		return nil
	}

	// Show the plan first.
	plan.Print(writer)

	// Confirm unless --auto-approve.
	if !autoApprove {
		_, _ = fmt.Fprint(writer, "\n"+style.key("Apply these changes? (y/N): "))

		reader := bufio.NewReader(input)
		ans, _ := reader.ReadString('\n')

		ans = strings.TrimSpace(strings.ToLower(ans))

		if ans != "y" && ans != "yes" {
			_, _ = fmt.Fprintln(writer, style.warning("Apply cancelled."))

			return nil
		}
	}

	_, _ = fmt.Fprintln(writer)

	for _, hostPlan := range plan.HostPlans {
		_, _ = fmt.Fprintf(writer, "%s %s...\n", style.key("Applying to"), style.projectName(hostPlan.Host.Name))

		client, err := clientFactory.NewClient(hostPlan.Host)
		if err != nil {
			return fmt.Errorf("connecting to %s: %w", hostPlan.Host.Name, err)
		}

		applyErr := applyHostPlan(cfg, hostPlan, client, writer)
		if applyErr != nil {
			_ = client.Close()

			return applyErr
		}

		_ = client.Close()
	}

	totalHosts, totalProjects, addCount, modifyCount, deleteCount, unchangedCount := plan.Stats()
	_ = totalHosts
	_ = totalProjects
	_ = unchangedCount
	_, _ = fmt.Fprintf(
		writer,
		"\n%s %d file(s) synced (%s added, %s modified, %s deleted)\n",
		style.success("Apply complete!"),
		addCount+modifyCount+deleteCount,
		style.success(fmt.Sprintf("%d", addCount)),
		style.warning(fmt.Sprintf("%d", modifyCount)),
		style.danger(fmt.Sprintf("%d", deleteCount)),
	)

	return nil
}

func applyHostPlan(cfg *config.CmtConfig, hostPlan HostPlan, client remote.RemoteClient, writer io.Writer) error {
	style := newOutputStyle(writer)

	for _, projectPlan := range hostPlan.Projects {
		hasChanges := false

		for _, filePlan := range projectPlan.Files {
			if filePlan.Action != ActionUnchanged {
				hasChanges = true

				break
			}
		}

		if !hasChanges {
			_, _ = fmt.Fprintf(writer, "  %s: %s\n", style.projectName(projectPlan.ProjectName), style.muted("no changes"))

			continue
		}

		_, _ = fmt.Fprintf(writer, "  %s:\n", style.projectName(projectPlan.ProjectName))

		// Create pre-configured directories.
		for _, dirPlan := range projectPlan.Dirs {
			if dirPlan.Exists {
				continue
			}

			_, _ = fmt.Fprintf(writer, "    %s %s/... ", style.key("creating dir"), dirPlan.RelativePath)

			err := client.MkdirAll(dirPlan.RemotePath)
			if err != nil {
				_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

				return fmt.Errorf("creating directory %s: %w", dirPlan.RemotePath, err)
			}

			_, _ = fmt.Fprintln(writer, style.success("done"))
		}

		// Collect managed files for manifest.
		localFiles := make(map[string]string)

		for _, filePlan := range projectPlan.Files {
			switch filePlan.Action {
			case ActionAdd, ActionModify:
				_, _ = fmt.Fprintf(writer, "    %s %s... ", style.key("uploading"), filePlan.RelativePath)

				err := client.WriteFile(filePlan.RemotePath, filePlan.LocalData)
				if err != nil {
					_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

					return fmt.Errorf("writing %s: %w", filePlan.RemotePath, err)
				}

				_, _ = fmt.Fprintln(writer, style.success("done"))

				localFiles[filePlan.RelativePath] = filePlan.LocalPath

			case ActionDelete:
				_, _ = fmt.Fprintf(writer, "    %s %s... ", style.key("deleting"), filePlan.RelativePath)

				err := client.Remove(filePlan.RemotePath)
				if err != nil {
					_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

					return fmt.Errorf("deleting %s: %w", filePlan.RemotePath, err)
				}

				_, _ = fmt.Fprintln(writer, style.success("done"))

			case ActionUnchanged:
				localFiles[filePlan.RelativePath] = filePlan.LocalPath
			}
		}

		// Write updated manifest.
		manifest := BuildManifest(localFiles)

		manifestData, marshalErr := json.MarshalIndent(manifest, "", "  ")
		if marshalErr != nil {
			return fmt.Errorf("marshalling manifest: %w", marshalErr)
		}

		manifestPath := path.Join(projectPlan.RemoteDir, manifestFile)

		err := client.WriteFile(manifestPath, manifestData)
		if err != nil {
			return fmt.Errorf("writing manifest: %w", err)
		}

		// Run post-sync command.
		if projectPlan.PostSyncCommand != "" {
			_, _ = fmt.Fprintf(writer, "    %s... ", style.key("running post-sync command"))

			out, err := client.RunCommand(projectPlan.RemoteDir, projectPlan.PostSyncCommand)
			if err != nil {
				_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

				if out != "" {
					_, _ = fmt.Fprintf(writer, "    %s %s\n", style.key("output:"), out)
				}

				return fmt.Errorf("post-sync command on %s/%s: %w", hostPlan.Host.Name, projectPlan.ProjectName, err)
			}

			_, _ = fmt.Fprintln(writer, style.success("done"))

			if out != "" {
				for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
					_, _ = fmt.Fprintf(writer, "      %s\n", line)
				}
			}
		}
	}

	return nil
}
