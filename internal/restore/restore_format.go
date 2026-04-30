package restore

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
)

func marshalRestoreJSON(value interface{}) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

func confirmRestoreRun(rt Env, report *restoreRunReport) (bool, error) {
	reader, interactive := runtimeStdinReader(rt)
	if !interactive {
		return false, NewRequestError("restore run requires --yes when not running interactively")
	}
	fmt.Fprintf(os.Stdout, "Restore revision %d into %s? [y/N]: ", report.Revision, report.Workspace)
	answer, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(answer) == "" {
		return false, fmt.Errorf("failed to read restore confirmation: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if restoreAnswerCancels(answer) {
		return false, ErrRestoreCancelled
	}
	return answer == "y" || answer == "yes", nil
}

func formatRestorePlan(report *restorePlanReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Restore plan for %s/%s\n", report.Label, report.Target)
	writeRestoreLines(&b, []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Storage", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Read Only", Value: "true"},
		{Label: "Executes Restore", Value: "false"},
	})
	writeRestoreResolvedSection(&b, report.ConfigFile, report.SourcePath, report.Storage, report.SecretsFile)
	writeRestoreSection(&b, "Safe Workspace", []SummaryLine{
		{Label: "Workspace", Value: report.Workspace},
		{Label: "Snapshot ID", Value: report.SnapshotID},
		{Label: "Rule", Value: "restore here first, never directly over the live source path"},
	})

	revision := "Not known from state"
	if report.LatestRevision > 0 {
		revision = strconv.Itoa(report.LatestRevision)
		if report.LatestRevisionAt != "" {
			revision = fmt.Sprintf("%s (%s)", revision, report.LatestRevisionAt)
		}
	}
	writeRestoreSection(&b, "Revision Signal", []SummaryLine{
		{Label: "State File", Value: report.StateFile},
		{Label: "State", Value: report.StateStatus},
		{Label: "Latest Revision", Value: revision},
		{Label: "Live Listing", Value: "run duplicacy list from the drill workspace"},
	})
	writeRestoreSection(&b, "Suggested Commands", []SummaryLine{
		{Label: "Create Workspace", Value: "mkdir -p " + shellQuote(report.Workspace)},
		{Label: "Enter Workspace", Value: "cd " + shellQuote(report.Workspace)},
		{Label: "Init Workspace", Value: report.InitCommand},
		{Label: "List Revisions", Value: report.ListCommand},
		{Label: "List Files", Value: report.ListFilesCommand},
		{Label: "Full Restore", Value: report.FullRestore},
		{Label: "Selective Restore", Value: report.SelectiveRestore},
		{Label: "Copy Back Preview", Value: report.CopyBackPreview},
	})
	writeRestoreSafetySection(&b, "inspect restored data first; use rsync --dry-run before live changes", report.DocumentationPath)
	return b.String()
}

func formatRestoreRevisions(report *restoreRevisionsReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Restore revision list for %s/%s\n", report.Label, report.Target)
	writeRestoreLines(&b, []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Storage", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Read Only", Value: "true"},
		{Label: "Executes Restore", Value: "false"},
	})
	writeRestoreResolvedSection(&b, report.ConfigFile, report.SourcePath, report.Storage, report.SecretsFile)
	writeRestoreSection(&b, "Workspace", []SummaryLine{
		{Label: "Mode", Value: report.WorkspaceMode},
		{Label: "Path", Value: report.Workspace},
	})
	writeRestoreSection(&b, "Revisions", []SummaryLine{
		{Label: "Revision Count", Value: strconv.Itoa(report.RevisionCount)},
		{Label: "Showing", Value: fmt.Sprintf("%d of %d", report.Showing, report.RevisionCount)},
		{Label: "Limit", Value: strconv.Itoa(report.Limit)},
	})
	for _, revision := range report.Revisions {
		value := strconv.Itoa(revision.Revision)
		if revision.CreatedAt != "" {
			value += " (" + revision.CreatedAt + ")"
		}
		writeRestoreLines(&b, []SummaryLine{{Label: "Revision", Value: value}})
	}
	writeRestoreSafetySection(&b, "not applicable; this command only lists revisions", "docs/restore-drills.md")
	return b.String()
}

func formatRestoreRun(report *restoreRunReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Restore run for %s/%s revision %d\n", report.Label, report.Target, report.Revision)
	writeRestoreLines(&b, []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Storage", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Executes Restore", Value: fmt.Sprintf("%t", !report.DryRun)},
		{Label: "Copies Back", Value: "false"},
	})
	writeRestoreResolvedSection(&b, report.ConfigFile, report.SourcePath, report.Storage, "")
	writeRestoreSection(&b, "Workspace", []SummaryLine{
		{Label: "Path", Value: report.Workspace},
		{Label: "Prepared", Value: fmt.Sprintf("%t", report.WorkspacePrepared)},
		{Label: "Rule", Value: "restored files stay in the workspace until an operator manually copies them back"},
	})
	writeRestoreSection(&b, "Restore", []SummaryLine{
		{Label: "Revision", Value: strconv.Itoa(report.Revision)},
		{Label: "Path", Value: report.RestorePath},
		{Label: "Command", Value: report.Command},
		{Label: "Dry Run", Value: fmt.Sprintf("%t", report.DryRun)},
		{Label: "Result", Value: report.Result},
	})
	if report.Output != "" {
		writeRestoreSection(&b, "Duplicacy Summary", []SummaryLine{{Label: "Output", Value: report.Output}})
	}
	writeRestoreSection(&b, "Safety", []SummaryLine{
		{Label: "Live Source", Value: "not modified"},
		{Label: "Copy Back", Value: "manual only; inspect restored data and use rsync --dry-run first"},
		{Label: "Guide", Value: report.Guide},
	})
	return b.String()
}

func formatRestoreBatchRun(report *restoreBatchRunReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Restore run for %s/%s revision %d\n", report.Label, report.Target, report.Revision)
	writeRestoreLines(&b, []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Storage", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Executes Restore", Value: "true"},
		{Label: "Copies Back", Value: "false"},
	})
	writeRestoreSection(&b, "Workspace", []SummaryLine{
		{Label: "Path", Value: report.Workspace},
		{Label: "Rule", Value: "restored files stay in the workspace until an operator manually copies them back"},
	})
	selectionLines := []SummaryLine{{Label: "Revision", Value: strconv.Itoa(report.Revision)}}
	for _, restorePath := range restoreDisplayPaths(report.RestorePaths) {
		selectionLines = append(selectionLines, SummaryLine{Label: "Path", Value: restorePath})
	}
	writeRestoreSection(&b, "Selection", selectionLines)
	resultLines := make([]SummaryLine, 0, len(report.Results))
	for i, result := range report.Results {
		resultLines = append(resultLines, SummaryLine{Label: fmt.Sprintf("Path %d", i+1), Value: fmt.Sprintf("%s - %s", restoreProgressPath(result.Path), result.Result)})
		if result.Output != "" {
			resultLines = append(resultLines, SummaryLine{Label: fmt.Sprintf("Output %d", i+1), Value: result.Output})
		}
	}
	writeRestoreSection(&b, "Results", resultLines)
	writeRestoreSection(&b, "Safety", []SummaryLine{
		{Label: "Live Source", Value: "not modified"},
		{Label: "Copy Back", Value: "manual only; inspect restored data and use rsync --dry-run first"},
		{Label: "Guide", Value: report.Guide},
	})
	return b.String()
}

func formatRestoreInspect(report *restoreInspectReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Restore inspection for %s/%s revision %d\n", report.Label, report.Target, report.Revision)
	writeRestoreLines(&b, []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Storage", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Read Only", Value: "true"},
		{Label: "Executes Restore", Value: "false"},
	})
	writeRestoreResolvedSection(&b, report.ConfigFile, report.SourcePath, report.Storage, "")
	writeRestoreSection(&b, "Inspection", []SummaryLine{
		{Label: "Revision", Value: strconv.Itoa(report.Revision)},
		{Label: "Path Prefix", Value: report.PathPrefix},
		{Label: "Mode", Value: "browse the revision contents and quit when done"},
	})
	writeRestoreSection(&b, "Safety", []SummaryLine{
		{Label: "Generated Commands", Value: "none; inspect mode does not generate restore commands"},
		{Label: "Restore Execution", Value: "not performed by this command"},
		{Label: "Guide", Value: report.Guide},
	})
	return b.String()
}

func formatRestoreSelect(report *restoreSelectReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Restore selection for %s/%s\n", report.Label, report.Target)
	writeRestoreLines(&b, []SummaryLine{
		{Label: "Label", Value: report.Label},
		{Label: "Storage", Value: report.Target},
		{Label: "Location", Value: report.Location},
		{Label: "Executes Restore", Value: "after confirmation"},
		{Label: "Copies Back", Value: "false"},
	})
	writeRestoreResolvedSection(&b, report.ConfigFile, report.SourcePath, report.Storage, "")
	writeRestoreSection(&b, "Workspace", []SummaryLine{
		{Label: "Path", Value: report.Workspace},
		{Label: "Prepared", Value: fmt.Sprintf("%t", report.WorkspacePrepared)},
	})
	selectionLines := []SummaryLine{{Label: "Revision", Value: strconv.Itoa(report.Revision)}}
	for _, restorePath := range restoreDisplayPaths(report.RestorePaths) {
		selectionLines = append(selectionLines, SummaryLine{Label: "Path", Value: restorePath})
	}
	writeRestoreSection(&b, "Selection", selectionLines)
	commandLines := make([]SummaryLine, 0, len(report.RestoreCommands))
	for _, command := range report.RestoreCommands {
		commandLines = append(commandLines, SummaryLine{Label: "Restore Command", Value: command})
	}
	writeRestoreSection(&b, "Generated Commands", commandLines)
	writeRestoreSection(&b, "Safety", []SummaryLine{
		{Label: "Command Model", Value: "restore select previews explicit restore run commands; restore run prepares the workspace and restores only there"},
		{Label: "Restore Execution", Value: "not performed unless you confirm after reviewing the commands"},
		{Label: "Copy Back", Value: "manual only; inspect restored data and use rsync --dry-run first"},
		{Label: "Guide", Value: report.Guide},
	})
	return b.String()
}

func restoreDisplayPaths(restorePaths []string) []string {
	restorePaths = normaliseRestoreSelection(restorePaths)
	display := make([]string, 0, len(restorePaths))
	for _, restorePath := range restorePaths {
		if restorePath == "" {
			display = append(display, "<full revision>")
			continue
		}
		display = append(display, restorePath)
	}
	return display
}

func writeRestoreResolvedSection(b *strings.Builder, configFile, sourcePath, storage, secretsFile string) {
	sourceDisplay := sourcePath
	if strings.TrimSpace(sourceDisplay) == "" {
		sourceDisplay = "Not configured (restore-only access is allowed; copy-back context unavailable)"
	}
	writeRestoreSection(b, "Resolved", []SummaryLine{
		{Label: "Config File", Value: configFile},
		{Label: "Source Path", Value: sourceDisplay},
		{Label: "Storage", Value: storage},
		{Label: "Secrets File", Value: secretsFile},
	})
}

func writeRestoreSafetySection(b *strings.Builder, copyBack, guide string) {
	writeRestoreSection(b, "Safety", []SummaryLine{
		{Label: "Restore Execution", Value: "not performed by this command"},
		{Label: "Copy Back", Value: copyBack},
		{Label: "Guide", Value: guide},
	})
}

func writeRestoreSection(b *strings.Builder, name string, lines []SummaryLine) {
	enableColour := logger.ColourEnabled(os.Stdout)
	fmt.Fprintf(b, "  Section: %s\n", name)
	for _, line := range lines {
		if strings.TrimSpace(line.Value) == "" {
			continue
		}
		fmt.Fprintf(b, "    %-18s : %s\n", line.Label, presentation.ColourizeSemanticValue(line.Value, enableColour))
	}
}

func writeRestoreLines(b *strings.Builder, lines []SummaryLine) {
	enableColour := logger.ColourEnabled(os.Stdout)
	for _, line := range lines {
		if strings.TrimSpace(line.Value) == "" {
			continue
		}
		fmt.Fprintf(b, "  %-20s : %s\n", line.Label, presentation.ColourizeSemanticValue(line.Value, enableColour))
	}
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
