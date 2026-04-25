package workflow

import (
	"bufio"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/restorepicker"
)

func newRestoreSelectContext(req *RestoreRequest, meta Metadata, rt Runtime, deps RestoreDeps) (*restoreExecutionContext, func(), error) {
	planner := NewConfigPlanner(meta, rt)
	plan := planner.derivePlan(req.ConfigRequest())
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return nil, func() {}, err
	}
	plan.applyConfig(cfg, rt)

	listingReq := *req
	if strings.TrimSpace(req.Workspace) != "" && restoreWorkspacePrepared(resolvedRestoreWorkspace(req, plan, deps)) {
		listingReq.Workspace = resolvedRestoreWorkspace(req, plan, deps)
	} else {
		listingReq.Workspace = ""
	}
	ctx, err := newRestoreExecutionContext(&listingReq, meta, rt, true, deps)
	if err != nil {
		return nil, func() {}, err
	}
	return ctx, ctx.cleanup, nil
}

func cleanRestorePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.ContainsRune(value, 0) {
		return "", NewRequestError("--path must not contain NUL characters")
	}
	if filepath.IsAbs(value) {
		return "", NewRequestError("--path must be relative to the backup snapshot: %s", value)
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", NewRequestError("--path must stay inside the backup snapshot: %s", value)
	}
	return filepath.ToSlash(cleaned), nil
}

func promptRestoreRevision(reader *bufio.Reader, revisions []duplicacy.RevisionInfo, limit int, deps RestoreDeps) (duplicacy.RevisionInfo, error) {
	shown := revisions
	if limit > 0 && len(shown) > limit {
		shown = shown[:limit]
	}
	fmt.Fprintln(deps.PromptOutput, "Available restore points:")
	for i, revision := range shown {
		fmt.Fprintf(deps.PromptOutput, "  %d. %s\n", i+1, formatRestorePointChoice(revision))
	}
	answer, err := promptRestoreLine(reader, "Select restore point by list number or revision id: ", deps)
	if err != nil {
		return duplicacy.RevisionInfo{}, err
	}
	choice, err := strconv.Atoi(strings.TrimSpace(answer))
	if err != nil || choice <= 0 {
		return duplicacy.RevisionInfo{}, NewRequestError("restore select requires a positive revision selection")
	}
	if choice <= len(shown) {
		return shown[choice-1], nil
	}
	for _, revision := range revisions {
		if revision.Revision == choice {
			return revision, nil
		}
	}
	return duplicacy.RevisionInfo{}, NewRequestError("revision %d was not found in the visible revision list", choice)
}

func formatRestorePointChoice(revision duplicacy.RevisionInfo) string {
	if createdAt := formatRevisionCreatedAt(revision); createdAt != "" {
		return fmt.Sprintf("%s | rev %d", createdAt, revision.Revision)
	}
	return fmt.Sprintf("rev %d", revision.Revision)
}

func promptRestoreSelectIntent(reader *bufio.Reader, pathPrefix string, deps RestoreDeps) (restoreSelectIntent, error) {
	fmt.Fprintln(deps.PromptOutput, "Choose what you want to do next:")
	fmt.Fprintln(deps.PromptOutput, "  1. Inspect revision contents only")
	if strings.TrimSpace(pathPrefix) == "" {
		fmt.Fprintln(deps.PromptOutput, "  2. Restore the full revision into the drill workspace")
	} else {
		fmt.Fprintf(deps.PromptOutput, "  2. Restore the full subtree under %q into the drill workspace\n", pathPrefix)
	}
	fmt.Fprintln(deps.PromptOutput, "  3. Restore selected files or directories into the drill workspace")
	answer, err := promptRestoreLine(reader, "Choose action [1-3]: ", deps)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "1", "inspect", "i":
		return restoreSelectIntentInspect, nil
	case "2", "full", "f":
		return restoreSelectIntentFull, nil
	case "3", "select", "selective", "s":
		return restoreSelectIntentSelective, nil
	default:
		return "", NewRequestError("restore select requires action 1, 2, or 3")
	}
}

func promptRestoreInspect(ctx *restoreExecutionContext, req *RestoreRequest, meta Metadata, revision int, deps RestoreDeps) error {
	pathPrefix, err := cleanRestorePath(req.PathPrefix)
	if err != nil {
		return err
	}
	output, err := ctx.dup.ListRevisionFiles(revision)
	if err != nil {
		return err
	}
	paths := extractRestoreFilePaths(output)
	if len(paths) == 0 {
		return NewRequestError("restore select found no restorable paths in revision %d", revision)
	}
	if pathPrefix != "" && !restorePathPrefixHasMatches(paths, pathPrefix) {
		return NewRequestError("restore select found no paths under prefix %q in revision %d", pathPrefix, revision)
	}
	if err := deps.RunInspectPicker(paths, restorepicker.AppOptions{
		Title:      fmt.Sprintf("Restore inspection for %s/%s", req.Label, req.Target()),
		PathPrefix: pathPrefix,
		Primitive: restorepicker.PrimitiveOptions{
			ScriptName: meta.ScriptName,
			Source:     req.Label,
			Target:     req.Target(),
			Revision:   strconv.Itoa(revision),
			Workspace:  ctx.workspace,
		},
	}); err != nil {
		if errors.Is(err, restorepicker.ErrPickerCancelled) {
			return NewRequestError("restore select cancelled")
		}
		return err
	}
	return nil
}

func promptRestorePath(ctx *restoreExecutionContext, req *RestoreRequest, meta Metadata, revision int, deps RestoreDeps) ([]string, error) {
	pathPrefix, err := cleanRestorePath(req.PathPrefix)
	if err != nil {
		return nil, err
	}
	output, err := ctx.dup.ListRevisionFiles(revision)
	if err != nil {
		return nil, err
	}
	paths := extractRestoreFilePaths(output)
	if len(paths) == 0 {
		return nil, NewRequestError("restore select found no restorable paths in revision %d", revision)
	}
	if pathPrefix != "" && !restorePathPrefixHasMatches(paths, pathPrefix) {
		return nil, NewRequestError("restore select found no paths under prefix %q in revision %d", pathPrefix, revision)
	}
	rootPath, rootIsDir := restoreSelectionRoot(paths, pathPrefix)
	restorePaths, err := deps.RunSelectPicker(paths, restorepicker.AppOptions{
		Title:      fmt.Sprintf("Restore selection for %s/%s", req.Label, req.Target()),
		PathPrefix: pathPrefix,
		Primitive: restorepicker.PrimitiveOptions{
			ScriptName: meta.ScriptName,
			Source:     req.Label,
			Target:     req.Target(),
			Revision:   strconv.Itoa(revision),
			Workspace:  ctx.workspace,
			RootPath:   rootPath,
			RootIsDir:  rootIsDir,
		},
	})
	if err != nil {
		if errors.Is(err, restorepicker.ErrPickerCancelled) {
			return nil, NewRequestError("restore select cancelled")
		}
		return nil, err
	}
	if len(restorePaths) == 0 {
		return nil, NewRequestError("restore select requires at least one restore path")
	}
	return restorePaths, nil
}

func restoreSelectionRoot(paths []string, pathPrefix string) (string, bool) {
	pathPrefix = strings.Trim(strings.TrimSpace(pathPrefix), "/")
	if pathPrefix == "" {
		return "", false
	}
	for _, path := range paths {
		path = strings.Trim(filepath.ToSlash(path), "/")
		if path == pathPrefix {
			return pathPrefix, false
		}
		if strings.HasPrefix(path, pathPrefix+"/") {
			return pathPrefix, true
		}
	}
	return pathPrefix, true
}

func restoreScopedDirectoryPattern(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	return path + "/*"
}

func restorePathPrefixHasMatches(paths []string, prefix string) bool {
	prefix = strings.Trim(prefix, "/")
	for _, path := range paths {
		path = strings.Trim(filepath.ToSlash(path), "/")
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func promptRestoreYesNo(reader restoreLineReader, prompt string, deps RestoreDeps) (bool, error) {
	answer, err := promptRestoreLine(reader, prompt, deps)
	if err != nil {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

func confirmRestoreSelectExecution(reader restoreLineReader, report *restoreSelectReport, deps RestoreDeps) (bool, error) {
	fmt.Fprintln(deps.PromptOutput, "Review the generated restore command(s):")
	fmt.Fprintf(deps.PromptOutput, "  Revision : %d\n", report.Revision)
	fmt.Fprintf(deps.PromptOutput, "  Workspace: %s\n", report.Workspace)
	for _, path := range restoreDisplayPaths(report.RestorePaths) {
		fmt.Fprintf(deps.PromptOutput, "  Path     : %s\n", path)
	}
	for _, command := range report.RestoreCommands {
		fmt.Fprintf(deps.PromptOutput, "  Command  : %s\n", command)
	}
	if report.WorkspacePrepared {
		return promptRestoreYesNo(reader, "Restore into this drill workspace now? [y/N]: ", deps)
	}
	return promptRestoreYesNo(reader, "Prepare this drill workspace and restore now? [y/N]: ", deps)
}

func promptRestoreLine(reader restoreLineReader, prompt string, deps RestoreDeps) (string, error) {
	fmt.Fprint(deps.PromptOutput, prompt)
	answer, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(answer) == "" {
		return "", fmt.Errorf("failed to read restore selection: %w", err)
	}
	return strings.TrimSpace(answer), nil
}

func buildRestoreRunCommand(scriptName string, req *RestoreRequest, revision int, restorePath string, workspace string) string {
	args := []string{
		"sudo",
		shellQuote(scriptName),
		"restore",
		"run",
		"--target",
		shellQuote(req.Target()),
		"--revision",
		strconv.Itoa(revision),
		"--workspace",
		shellQuote(workspace),
		"--yes",
	}
	if restorePath != "" {
		args = append(args, "--path", shellQuote(restorePath))
	}
	args = appendRestoreConfigFlags(args, req)
	args = append(args, shellQuote(req.Label))
	return strings.Join(args, " ")
}

func buildRestoreRunCommands(scriptName string, req *RestoreRequest, revision int, restorePaths []string, workspace string) []string {
	restorePaths = normaliseRestoreSelection(restorePaths)
	commands := make([]string, 0, len(restorePaths))
	for _, restorePath := range restorePaths {
		commands = append(commands, buildRestoreRunCommand(scriptName, req, revision, restorePath, workspace))
	}
	return commands
}

func normaliseRestoreSelection(restorePaths []string) []string {
	if len(restorePaths) == 0 {
		return []string{""}
	}
	normalised := make([]string, 0, len(restorePaths))
	for _, restorePath := range restorePaths {
		normalised = append(normalised, strings.TrimSpace(restorePath))
	}
	return normalised
}

func appendRestoreConfigFlags(args []string, req *RestoreRequest) []string {
	if strings.TrimSpace(req.ConfigDir) != "" {
		args = append(args, "--config-dir", shellQuote(req.ConfigDir))
	}
	if strings.TrimSpace(req.SecretsDir) != "" {
		args = append(args, "--secrets-dir", shellQuote(req.SecretsDir))
	}
	return args
}
