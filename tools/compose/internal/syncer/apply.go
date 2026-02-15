package syncer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
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
func ApplyWithDeps(
	cfg *config.CmtConfig,
	plan *SyncPlan,
	autoApprove bool,
	writer io.Writer,
	deps ApplyDependencies,
) error {
	style := newOutputStyle(writer)
	clientFactory, input := resolveApplyDependencies(deps)

	if !plan.HasChanges() {
		_, _ = fmt.Fprintln(writer, style.muted("No changes to apply."))

		return nil
	}

	// Show the plan first.
	plan.Print(writer)

	if !autoApprove && !confirmApply(input, writer, style) {
		_, _ = fmt.Fprintln(writer, style.warning("Apply cancelled."))

		return nil
	}

	_, _ = fmt.Fprintln(writer)

	err := applyAllHosts(cfg, plan, clientFactory, writer, style)
	if err != nil {
		return err
	}

	printApplySummary(plan, writer, style)

	return nil
}

func applyHostPlan(cfg *config.CmtConfig, hostPlan HostPlan, client remote.RemoteClient, writer io.Writer) error {
	style := newOutputStyle(writer)

	for _, projectPlan := range hostPlan.Projects {
		if !projectHasChanges(projectPlan) {
			_, _ = fmt.Fprintf(writer, "  %s: %s\n", style.projectName(projectPlan.ProjectName), style.muted("no changes"))

			continue
		}

		_, _ = fmt.Fprintf(writer, "  %s:\n", style.projectName(projectPlan.ProjectName))

		err := applyProjectPlan(cfg, hostPlan, projectPlan, client, writer, style)
		if err != nil {
			return err
		}
	}

	return nil
}

func resolveApplyDependencies(deps ApplyDependencies) (remote.ClientFactory, io.Reader) {
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

	return clientFactory, input
}

func confirmApply(input io.Reader, writer io.Writer, style outputStyle) bool {
	_, _ = fmt.Fprint(writer, "\n"+style.key("Apply these changes? (y/N): "))

	reader := bufio.NewReader(input)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	return answer == "y" || answer == "yes"
}

func applyAllHosts(
	cfg *config.CmtConfig,
	plan *SyncPlan,
	clientFactory remote.ClientFactory,
	writer io.Writer,
	style outputStyle,
) error {
	for _, hostPlan := range plan.HostPlans {
		_, _ = fmt.Fprintf(writer, "%s %s...\n", style.key("Applying to"), style.projectName(hostPlan.Host.Name))

		client, err := clientFactory.NewClient(hostPlan.Host)
		if err != nil {
			return fmt.Errorf("connecting to %s: %w", hostPlan.Host.Name, err)
		}

		applyErr := applyHostPlan(cfg, hostPlan, client, writer)
		_ = client.Close()

		if applyErr != nil {
			return applyErr
		}
	}

	return nil
}

func printApplySummary(plan *SyncPlan, writer io.Writer, style outputStyle) {
	hostCount, projectCount, addCount, modifyCount, deleteCount, unchangedCount := plan.Stats()
	_ = hostCount
	_ = projectCount
	_ = unchangedCount

	_, _ = fmt.Fprintf(
		writer,
		"\n%s %d file(s) synced (%s added, %s modified, %s deleted)\n",
		style.success("Apply complete!"),
		addCount+modifyCount+deleteCount,
		style.success(strconv.Itoa(addCount)),
		style.warning(strconv.Itoa(modifyCount)),
		style.danger(strconv.Itoa(deleteCount)),
	)
}

func projectHasChanges(projectPlan ProjectPlan) bool {
	for _, filePlan := range projectPlan.Files {
		if filePlan.Action != ActionUnchanged {
			return true
		}
	}

	return false
}

func applyProjectPlan(
	_ *config.CmtConfig,
	hostPlan HostPlan,
	projectPlan ProjectPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) error {
	err := createMissingDirs(projectPlan, client, writer, style)
	if err != nil {
		return err
	}

	localFiles, err := syncProjectFiles(projectPlan, client, writer, style)
	if err != nil {
		return err
	}

	err = writeProjectManifest(projectPlan.RemoteDir, localFiles, client)
	if err != nil {
		return err
	}

	err = runPostSyncCommand(hostPlan, projectPlan, client, writer, style)
	if err != nil {
		return err
	}

	return nil
}

func createMissingDirs(projectPlan ProjectPlan, client remote.RemoteClient, writer io.Writer, style outputStyle) error {
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

	return nil
}

func syncProjectFiles(
	projectPlan ProjectPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) (map[string]string, error) {
	localFiles := make(map[string]string)

	for _, filePlan := range projectPlan.Files {
		switch filePlan.Action {
		case ActionAdd, ActionModify:
			_, _ = fmt.Fprintf(writer, "    %s %s... ", style.key("uploading"), filePlan.RelativePath)

			err := client.WriteFile(filePlan.RemotePath, filePlan.LocalData)
			if err != nil {
				_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

				return nil, fmt.Errorf("writing %s: %w", filePlan.RemotePath, err)
			}

			_, _ = fmt.Fprintln(writer, style.success("done"))
			localFiles[filePlan.RelativePath] = filePlan.LocalPath
		case ActionDelete:
			_, _ = fmt.Fprintf(writer, "    %s %s... ", style.key("deleting"), filePlan.RelativePath)

			err := client.Remove(filePlan.RemotePath)
			if err != nil {
				_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

				return nil, fmt.Errorf("deleting %s: %w", filePlan.RemotePath, err)
			}

			_, _ = fmt.Fprintln(writer, style.success("done"))
		case ActionUnchanged:
			localFiles[filePlan.RelativePath] = filePlan.LocalPath
		}
	}

	return localFiles, nil
}

func writeProjectManifest(remoteDir string, localFiles map[string]string, client remote.RemoteClient) error {
	manifest := BuildManifest(localFiles)

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling manifest: %w", err)
	}

	manifestPath := path.Join(remoteDir, manifestFile)

	err = client.WriteFile(manifestPath, manifestData)
	if err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	return nil
}

func runPostSyncCommand(
	hostPlan HostPlan,
	projectPlan ProjectPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) error {
	if projectPlan.PostSyncCommand == "" {
		return nil
	}

	_, _ = fmt.Fprintf(writer, "    %s... ", style.key("running post-sync command"))

	output, err := client.RunCommand(projectPlan.RemoteDir, projectPlan.PostSyncCommand)
	if err != nil {
		_, _ = fmt.Fprintln(writer, style.danger("FAILED"))
		if output != "" {
			_, _ = fmt.Fprintf(writer, "    %s %s\n", style.key("output:"), output)
		}

		return fmt.Errorf("post-sync command on %s/%s: %w", hostPlan.Host.Name, projectPlan.ProjectName, err)
	}

	_, _ = fmt.Fprintln(writer, style.success("done"))

	if output == "" {
		return nil
	}

	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		_, _ = fmt.Fprintf(writer, "      %s\n", line)
	}

	return nil
}
