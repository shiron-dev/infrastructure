package syncer

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"cmt/internal/config"
	"cmt/internal/remote"
)

type ApplyDependencies struct {
	ClientFactory remote.ClientFactory
	Input         io.Reader
	HookRunner    HookRunner
	ConfigPath    string
}

func Apply(cfg *config.CmtConfig, plan *SyncPlan, autoApprove bool, w io.Writer) error {
	var dependencies ApplyDependencies

	return ApplyWithDeps(cfg, plan, autoApprove, false, w, dependencies)
}

func ApplyWithDeps(
	cfg *config.CmtConfig,
	plan *SyncPlan,
	autoApprove bool,
	refreshManifestOnNoop bool,
	writer io.Writer,
	deps ApplyDependencies,
) error {
	style := newOutputStyle(writer)
	clientFactory, input, hookRunner := resolveApplyDependencies(deps)

	handled, err := handleNoChanges(plan, refreshManifestOnNoop, clientFactory, writer, style)
	if handled || err != nil {
		return err
	}

	plan.Print(writer)

	cancelled, hookErr := runBeforePromptApplyHook(cfg, plan, deps, hookRunner, writer, style)
	if hookErr != nil {
		return hookErr
	}

	if cancelled {
		return nil
	}

	if confirmApplyOrCancel(autoApprove, input, writer, style) {
		return nil
	}

	cancelled, hookErr = runAfterPromptApplyHook(cfg, plan, deps, hookRunner, writer, style)
	if hookErr != nil {
		return hookErr
	}

	if cancelled {
		return nil
	}

	_, _ = fmt.Fprintln(writer)

	err = applyAllHosts(cfg, plan, clientFactory, writer, style)
	if err != nil {
		return err
	}

	printApplySummary(plan, writer, style)

	return nil
}

func runBeforePromptApplyHook(
	cfg *config.CmtConfig,
	plan *SyncPlan,
	deps ApplyDependencies,
	hookRunner HookRunner,
	writer io.Writer,
	style outputStyle,
) (bool, error) {
	if cfg.BeforeApplyHooks == nil {
		return false, nil
	}

	payload := buildBeforePromptPayload(plan, deps.ConfigPath, cfg.BasePath)

	return executeApplyHook(
		cfg.BeforeApplyHooks.BeforePrompt,
		payload,
		"beforePrompt",
		hookRunner,
		writer,
		style,
	)
}

func runAfterPromptApplyHook(
	cfg *config.CmtConfig,
	plan *SyncPlan,
	deps ApplyDependencies,
	hookRunner HookRunner,
	writer io.Writer,
	style outputStyle,
) (bool, error) {
	if cfg.BeforeApplyHooks == nil {
		return false, nil
	}

	payload := buildAfterPromptPayload(plan, deps.ConfigPath, cfg.BasePath)

	return executeApplyHook(
		cfg.BeforeApplyHooks.AfterPrompt,
		payload,
		"afterPrompt",
		hookRunner,
		writer,
		style,
	)
}

func handleNoChanges(
	plan *SyncPlan,
	refreshManifestOnNoop bool,
	clientFactory remote.ClientFactory,
	writer io.Writer,
	style outputStyle,
) (bool, error) {
	if plan.HasChanges() {
		return false, nil
	}

	_, _ = fmt.Fprintln(writer, style.muted("No changes to apply."))

	if !refreshManifestOnNoop {
		return true, nil
	}

	err := refreshManifestForAllHosts(plan, clientFactory, writer, style)
	if err != nil {
		return true, err
	}

	_, _ = fmt.Fprintln(writer, style.success("Manifest refreshed."))

	return true, nil
}

func executeApplyHook(
	hookCmd *config.HookCommand,
	payload any,
	hookName string,
	hookRunner HookRunner,
	writer io.Writer,
	style outputStyle,
) (bool, error) {
	result := runHook(hookCmd, payload, hookName, hookRunner, writer, style)
	switch result {
	case hookContinue:
		return false, nil
	case hookRejected:
		_, _ = fmt.Fprintln(writer, style.warning("Apply cancelled by hook."))

		return true, nil
	case hookError:
		return false, errors.New(hookName + " hook failed")
	}

	return false, errors.New("unknown hook result")
}

func confirmApplyOrCancel(autoApprove bool, input io.Reader, writer io.Writer, style outputStyle) bool {
	if autoApprove {
		return false
	}

	if confirmApply(input, writer, style) {
		return false
	}

	_, _ = fmt.Fprintln(writer, style.warning("Apply cancelled."))

	return true
}

func refreshManifestForAllHosts(
	plan *SyncPlan,
	clientFactory remote.ClientFactory,
	writer io.Writer,
	style outputStyle,
) error {
	for _, hostPlan := range plan.HostPlans {
		_, _ = fmt.Fprintf(
			writer,
			"%s %s...\n",
			style.key("Refreshing manifest on"),
			style.projectName(hostPlan.Host.Name),
		)

		client, err := clientFactory.NewClient(hostPlan.Host)
		if err != nil {
			return fmt.Errorf("connecting to %s: %w", hostPlan.Host.Name, err)
		}

		refreshErr := refreshHostManifest(hostPlan, client, writer, style)
		_ = client.Close()

		if refreshErr != nil {
			return refreshErr
		}
	}

	return nil
}

func refreshHostManifest(
	hostPlan HostPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) error {
	for _, projectPlan := range hostPlan.Projects {
		localFiles, maskHints := collectManifestInputs(projectPlan)

		_, _ = fmt.Fprintf(
			writer,
			"  %s... ",
			style.projectName(projectPlan.ProjectName),
		)

		err := writeProjectManifest(projectPlan.RemoteDir, localFiles, maskHints, client)
		if err != nil {
			_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

			return err
		}

		_, _ = fmt.Fprintln(writer, style.success("done"))
	}

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

func resolveApplyDependencies(deps ApplyDependencies) (remote.ClientFactory, io.Reader, HookRunner) {
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

	hookRunner := deps.HookRunner
	if hookRunner == nil {
		hookRunner = defaultHookRunner
	}

	return clientFactory, input, hookRunner
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
		"\n%s %d file(s) synced (%s added, %s modified, %s deleted)",
		style.success("Apply complete!"),
		addCount+modifyCount+deleteCount,
		style.success(strconv.Itoa(addCount)),
		style.warning(strconv.Itoa(modifyCount)),
		style.danger(strconv.Itoa(deleteCount)),
	)

	composeStart, composeStop := plan.ComposeStats()
	if composeStart > 0 || composeStop > 0 {
		_, _ = fmt.Fprintf(writer, ", compose: %s started, %s stopped",
			style.success(strconv.Itoa(composeStart)),
			style.danger(strconv.Itoa(composeStop)),
		)
	}

	_, _ = fmt.Fprintln(writer)
}

func projectHasChanges(projectPlan ProjectPlan) bool {
	for _, filePlan := range projectPlan.Files {
		if filePlan.Action != ActionUnchanged {
			return true
		}
	}

	return projectPlan.Compose.HasChanges()
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

	localFiles, maskHints, err := syncProjectFiles(projectPlan, client, writer, style)
	if err != nil {
		return err
	}

	err = writeProjectManifest(projectPlan.RemoteDir, localFiles, maskHints, client)
	if err != nil {
		return err
	}

	err = runPostSyncCommand(hostPlan, projectPlan, client, writer, style)
	if err != nil {
		return err
	}

	err = runComposeAction(hostPlan, projectPlan, client, writer, style)
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
) (map[string]string, map[string][]MaskHint, error) {
	localFiles, maskHints := collectManifestInputs(projectPlan)

	for _, filePlan := range projectPlan.Files {
		switch filePlan.Action {
		case ActionAdd, ActionModify:
			_, _ = fmt.Fprintf(writer, "    %s %s... ", style.key("uploading"), filePlan.RelativePath)

			err := client.WriteFile(filePlan.RemotePath, filePlan.LocalData)
			if err != nil {
				_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

				return nil, nil, fmt.Errorf("writing %s: %w", filePlan.RemotePath, err)
			}

			_, _ = fmt.Fprintln(writer, style.success("done"))

		case ActionDelete:
			_, _ = fmt.Fprintf(writer, "    %s %s... ", style.key("deleting"), filePlan.RelativePath)

			err := client.Remove(filePlan.RemotePath)
			if err != nil {
				_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

				return nil, nil, fmt.Errorf("deleting %s: %w", filePlan.RemotePath, err)
			}

			_, _ = fmt.Fprintln(writer, style.success("done"))
		case ActionUnchanged:
			// Manifest inputs are collected before this loop.
			continue
		}
	}

	return localFiles, maskHints, nil
}

func collectManifestInputs(projectPlan ProjectPlan) (map[string]string, map[string][]MaskHint) {
	localFiles := make(map[string]string)
	maskHints := make(map[string][]MaskHint)

	for _, filePlan := range projectPlan.Files {
		if filePlan.Action == ActionDelete {
			continue
		}

		localFiles[filePlan.RelativePath] = filePlan.LocalPath

		if len(filePlan.MaskHints) > 0 {
			maskHints[filePlan.RelativePath] = append([]MaskHint(nil), filePlan.MaskHints...)
		}
	}

	return localFiles, maskHints
}

func writeProjectManifest(
	remoteDir string,
	localFiles map[string]string,
	maskHints map[string][]MaskHint,
	client remote.RemoteClient,
) error {
	manifest := BuildManifestWithMaskHints(localFiles, maskHints)

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

func runComposeAction(
	hostPlan HostPlan,
	projectPlan ProjectPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) error {
	var cmd string

	switch projectPlan.Compose.ActionType {
	case ComposeNoChange:
		return nil
	case ComposeStartServices:
		cmd = "docker compose up -d"
	case ComposeStopServices:
		cmd = "docker compose down"
		if projectPlan.RemoveOrphans {
			cmd += " --remove-orphans"
		}
	default:
		return nil
	}

	_, _ = fmt.Fprintf(writer, "    %s %s... ", style.key("compose"), cmd)

	output, err := client.RunCommand(projectPlan.RemoteDir, cmd)
	if err != nil {
		_, _ = fmt.Fprintln(writer, style.danger("FAILED"))
		if output != "" {
			_, _ = fmt.Fprintf(writer, "    %s %s\n", style.key("output:"), output)
		}

		return fmt.Errorf("compose %s on %s/%s: %w", cmd, hostPlan.Host.Name, projectPlan.ProjectName, err)
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
