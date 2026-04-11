package workflow

import (
	"fmt"
	"strings"
)

type SummaryLine struct {
	Label string
	Value string
}

func OperationMode(req *Request) string {
	var parts []string
	if req.DoBackup {
		parts = append(parts, "Backup")
	}
	if req.DoPrune {
		if req.ForcePrune {
			parts = append(parts, "Forced prune")
		} else {
			parts = append(parts, "Safe prune")
		}
	}
	if req.DoCleanupStore {
		parts = append(parts, "Storage cleanup")
	}
	if req.FixPerms {
		parts = append(parts, "Fix permissions")
	}

	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts, " + ")
}

func SummaryLines(plan *Plan) []SummaryLine {
	if plan.FixPermsOnly {
		return []SummaryLine{
			{Label: "Operation Mode", Value: plan.OperationMode},
			SummaryLine{Label: "Destination", Value: plan.BackupTarget},
			SummaryLine{Label: "Local Owner", Value: plan.LocalOwner},
			SummaryLine{Label: "Local Group", Value: plan.LocalGroup},
			SummaryLine{Label: "Dry Run", Value: fmt.Sprintf("%t", plan.DryRun)},
		}
	}

	lines := []SummaryLine{
		{Label: "Operation Mode", Value: plan.OperationMode},
		SummaryLine{Label: "Config File", Value: plan.ConfigFile},
		SummaryLine{Label: "Target", Value: plan.TargetName()},
		SummaryLine{Label: "Mode", Value: plan.ModeDisplay},
		SummaryLine{Label: "Source", Value: plan.SnapshotSource},
	}
	if plan.DoBackup {
		lines = append(lines, SummaryLine{Label: "Snapshot", Value: plan.RepositoryPath})
	}
	lines = append(lines, SummaryLine{Label: "Destination", Value: plan.BackupTarget})

	if !plan.Verbose {
		if plan.DryRun {
			lines = append(lines, SummaryLine{Label: "Dry Run", Value: "true"})
		}
		if plan.ForcePrune {
			lines = append(lines, SummaryLine{Label: "Force Prune", Value: "true"})
		}
		if plan.DoCleanupStore {
			lines = append(lines, SummaryLine{Label: "Cleanup Storage", Value: "true"})
		}
		if plan.FixPerms {
			lines = append(lines,
				SummaryLine{Label: "Local Owner", Value: plan.LocalOwner},
				SummaryLine{Label: "Local Group", Value: plan.LocalGroup},
			)
		}
		return lines
	}

	lines = append(lines,
		SummaryLine{Label: "Backup Label", Value: plan.BackupLabel},
		SummaryLine{Label: "Work Dir", Value: plan.WorkDir()},
	)

	if plan.Threads > 0 {
		lines = append(lines, SummaryLine{Label: "Threads", Value: fmt.Sprintf("%d", plan.Threads)})
	} else {
		lines = append(lines, SummaryLine{Label: "Threads", Value: "<n/a>"})
	}

	if plan.Filter != "" {
		lines = append(lines, SummaryLine{Label: "Filter", Value: plan.Filter})
	} else {
		lines = append(lines, SummaryLine{Label: "Filter", Value: "<none>"})
	}

	if plan.PruneOptions != "" {
		lines = append(lines, SummaryLine{Label: "Prune Options", Value: plan.PruneOptions})
	} else {
		lines = append(lines, SummaryLine{Label: "Prune Options", Value: "<none>"})
	}

	lines = append(lines,
		SummaryLine{Label: "Log Retention", Value: fmt.Sprintf("%d", plan.LogRetentionDays)},
		SummaryLine{Label: "Dry Run", Value: fmt.Sprintf("%t", plan.DryRun)},
		SummaryLine{Label: "Force Prune", Value: fmt.Sprintf("%t", plan.ForcePrune)},
		SummaryLine{Label: "Cleanup Storage", Value: fmt.Sprintf("%t", plan.DoCleanupStore)},
		SummaryLine{Label: "Fix Perms", Value: fmt.Sprintf("%t", plan.FixPerms)},
		SummaryLine{Label: "Prune Max %", Value: fmt.Sprintf("%d", plan.SafePruneMaxDeletePercent)},
		SummaryLine{Label: "Prune Max Count", Value: fmt.Sprintf("%d", plan.SafePruneMaxDeleteCount)},
		SummaryLine{Label: "Prune Min Total Revs", Value: fmt.Sprintf("%d", plan.SafePruneMinTotalForPercent)},
	)

	if plan.FixPerms {
		lines = append(lines,
			SummaryLine{Label: "Local Owner", Value: plan.LocalOwner},
			SummaryLine{Label: "Local Group", Value: plan.LocalGroup},
		)
	}

	if (plan.TargetType == targetRemote || plan.TargetName() == targetRemote || plan.RemoteMode) && plan.Secrets != nil {
		lines = append(lines,
			SummaryLine{Label: "Secrets Dir", Value: plan.SecretsDir},
			SummaryLine{Label: "Secrets File", Value: plan.SecretsFile},
			SummaryLine{Label: "Remote Access Key", Value: plan.Secrets.MaskedID()},
			SummaryLine{Label: "Remote Secret Key", Value: plan.Secrets.MaskedSecret()},
		)
	}

	return lines
}
