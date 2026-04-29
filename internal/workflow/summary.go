package workflow

import (
	"fmt"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"
)

type SummaryLine = workflowcore.SummaryLine

func OperationMode(req *RuntimeRequest) string {
	if req == nil {
		return ""
	}
	switch req.Mode {
	case RuntimeModeBackup:
		return "Backup"
	case RuntimeModePrune:
		if req.ForcePrune {
			return "Forced prune"
		}
		return "Safe prune"
	case RuntimeModeCleanupStorage:
		return "Storage cleanup"
	default:
		return ""
	}
}

func SummaryLines(plan *Plan) []SummaryLine {
	lines := []SummaryLine{
		{Label: "Operation Mode", Value: plan.Request.OperationMode},
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Location", Value: plan.Config.Location},
		{Label: "Config File", Value: plan.Paths.ConfigFile},
		{Label: "Source Path", Value: plan.Paths.SnapshotSource},
	}
	if plan.Request.DoBackup {
		lines = append(lines, []SummaryLine{{Label: "Snapshot", Value: plan.Paths.RepositoryPath}}...)
	}
	lines = append(lines, []SummaryLine{{Label: "Storage", Value: plan.Paths.BackupTarget}}...)

	if !plan.Request.Verbose {
		if plan.Request.DryRun {
			lines = append(lines, []SummaryLine{{Label: "Dry Run", Value: "true"}}...)
		}
		if plan.Request.ForcePrune {
			lines = append(lines, []SummaryLine{{Label: "Force Prune", Value: "true"}}...)
		}
		if plan.Request.DoCleanupStore {
			lines = append(lines, []SummaryLine{{Label: "Cleanup Storage", Value: "true"}}...)
		}
		return lines
	}

	lines = append(lines, []SummaryLine{
		{Label: "Backup Label", Value: plan.Config.BackupLabel},
		{Label: "Work Dir", Value: plan.WorkDir()},
	}...)

	if plan.Config.Threads > 0 {
		lines = append(lines, []SummaryLine{{Label: "Threads", Value: fmt.Sprintf("%d", plan.Config.Threads)}}...)
	} else {
		lines = append(lines, []SummaryLine{{Label: "Threads", Value: "<n/a>"}}...)
	}

	if plan.Config.Filter != "" {
		lines = append(lines, []SummaryLine{{Label: "Filter", Value: plan.Config.Filter}}...)
	} else {
		lines = append(lines, []SummaryLine{{Label: "Filter", Value: "<none>"}}...)
	}

	if plan.Config.PruneOptions != "" {
		lines = append(lines, []SummaryLine{{Label: "Prune Options", Value: plan.Config.PruneOptions}}...)
	} else {
		lines = append(lines, []SummaryLine{{Label: "Prune Options", Value: "<none>"}}...)
	}

	lines = append(lines, []SummaryLine{
		{Label: "Log Retention", Value: fmt.Sprintf("%d", plan.Config.LogRetentionDays)},
		{Label: "Dry Run", Value: fmt.Sprintf("%t", plan.Request.DryRun)},
		{Label: "Force Prune", Value: fmt.Sprintf("%t", plan.Request.ForcePrune)},
		{Label: "Cleanup Storage", Value: fmt.Sprintf("%t", plan.Request.DoCleanupStore)},
		{Label: "Prune Max %", Value: fmt.Sprintf("%d", plan.Config.SafePruneMaxDeletePercent)},
		{Label: "Prune Max Count", Value: fmt.Sprintf("%d", plan.Config.SafePruneMaxDeleteCount)},
		{Label: "Prune Min Total Revs", Value: fmt.Sprintf("%d", plan.Config.SafePruneMinTotalForPercent)},
	}...)

	if plan.Secrets != nil {
		lines = append(lines, []SummaryLine{
			{Label: "Secrets Dir", Value: plan.Paths.SecretsDir},
			{Label: "Secrets File", Value: plan.Paths.SecretsFile},
			{Label: "Storage Keys", Value: plan.Secrets.MaskedKeys()},
		}...)
	}

	return lines
}
