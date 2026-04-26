package workflow

import (
	"fmt"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
)

type SummaryLine = presentation.Line

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
		{Label: "Operation Mode", Value: plan.OperationMode},
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Location", Value: plan.Location},
		{Label: "Config File", Value: plan.ConfigFile},
		{Label: "Source", Value: plan.SnapshotSource},
	}
	if plan.DoBackup {
		lines = append(lines, []SummaryLine{{Label: "Snapshot", Value: plan.RepositoryPath}}...)
	}
	lines = append(lines, []SummaryLine{{Label: "Storage", Value: plan.BackupTarget}}...)

	if !plan.Verbose {
		if plan.DryRun {
			lines = append(lines, []SummaryLine{{Label: "Dry Run", Value: "true"}}...)
		}
		if plan.ForcePrune {
			lines = append(lines, []SummaryLine{{Label: "Force Prune", Value: "true"}}...)
		}
		if plan.DoCleanupStore {
			lines = append(lines, []SummaryLine{{Label: "Cleanup Storage", Value: "true"}}...)
		}
		return lines
	}

	lines = append(lines, []SummaryLine{
		{Label: "Backup Label", Value: plan.BackupLabel},
		{Label: "Work Dir", Value: plan.WorkDir()},
	}...)

	if plan.Threads > 0 {
		lines = append(lines, []SummaryLine{{Label: "Threads", Value: fmt.Sprintf("%d", plan.Threads)}}...)
	} else {
		lines = append(lines, []SummaryLine{{Label: "Threads", Value: "<n/a>"}}...)
	}

	if plan.Filter != "" {
		lines = append(lines, []SummaryLine{{Label: "Filter", Value: plan.Filter}}...)
	} else {
		lines = append(lines, []SummaryLine{{Label: "Filter", Value: "<none>"}}...)
	}

	if plan.PruneOptions != "" {
		lines = append(lines, []SummaryLine{{Label: "Prune Options", Value: plan.PruneOptions}}...)
	} else {
		lines = append(lines, []SummaryLine{{Label: "Prune Options", Value: "<none>"}}...)
	}

	lines = append(lines, []SummaryLine{
		{Label: "Log Retention", Value: fmt.Sprintf("%d", plan.LogRetentionDays)},
		{Label: "Dry Run", Value: fmt.Sprintf("%t", plan.DryRun)},
		{Label: "Force Prune", Value: fmt.Sprintf("%t", plan.ForcePrune)},
		{Label: "Cleanup Storage", Value: fmt.Sprintf("%t", plan.DoCleanupStore)},
		{Label: "Prune Max %", Value: fmt.Sprintf("%d", plan.SafePruneMaxDeletePercent)},
		{Label: "Prune Max Count", Value: fmt.Sprintf("%d", plan.SafePruneMaxDeleteCount)},
		{Label: "Prune Min Total Revs", Value: fmt.Sprintf("%d", plan.SafePruneMinTotalForPercent)},
	}...)

	if plan.Secrets != nil {
		lines = append(lines, []SummaryLine{
			{Label: "Secrets Dir", Value: plan.SecretsDir},
			{Label: "Secrets File", Value: plan.SecretsFile},
			{Label: "Storage Keys", Value: plan.Secrets.MaskedKeys()},
		}...)
	}

	return lines
}
