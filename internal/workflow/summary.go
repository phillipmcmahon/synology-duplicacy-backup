package workflow

import "fmt"

type SummaryLine struct {
	Label string
	Value string
}

func OperationMode(req *Request) string {
	switch {
	case req.FixPermsOnly:
		return "Fix permissions only"
	case req.DoBackup && req.FixPerms:
		return "Backup + fix permissions"
	case req.DoBackup:
		return "Backup only"
	case req.DoPrune && req.DeepPruneMode && req.FixPerms:
		return "Prune deep + fix permissions"
	case req.DoPrune && req.DeepPruneMode:
		return "Prune deep"
	case req.DoPrune && req.FixPerms:
		return "Prune safe + fix permissions"
	case req.DoPrune:
		return "Prune safe"
	default:
		return ""
	}
}

func SummaryLines(plan *Plan) []SummaryLine {
	lines := []SummaryLine{{Label: "Operation Mode", Value: plan.OperationMode}}

	if plan.Request.FixPermsOnly {
		return append(lines,
			SummaryLine{Label: "Destination", Value: plan.BackupTarget},
			SummaryLine{Label: "Local Owner", Value: plan.Config.LocalOwner},
			SummaryLine{Label: "Local Group", Value: plan.Config.LocalGroup},
			SummaryLine{Label: "Dry Run", Value: fmt.Sprintf("%t", plan.Request.DryRun)},
		)
	}

	lines = append(lines,
		SummaryLine{Label: "Config File", Value: plan.ConfigFile},
		SummaryLine{Label: "Backup Label", Value: plan.BackupLabel},
		SummaryLine{Label: "Mode", Value: plan.ModeLabel()},
		SummaryLine{Label: "Source", Value: plan.SnapshotSource},
	)
	if plan.Request.DoBackup {
		lines = append(lines, SummaryLine{Label: "Snapshot", Value: plan.RepositoryPath})
	}
	lines = append(lines,
		SummaryLine{Label: "Work Dir", Value: plan.WorkDir()},
		SummaryLine{Label: "Destination", Value: plan.BackupTarget},
	)

	if plan.Config.Threads > 0 {
		lines = append(lines, SummaryLine{Label: "Threads", Value: fmt.Sprintf("%d", plan.Config.Threads)})
	} else {
		lines = append(lines, SummaryLine{Label: "Threads", Value: "<n/a>"})
	}

	if plan.Config.Filter != "" {
		lines = append(lines, SummaryLine{Label: "Filter", Value: plan.Config.Filter})
	} else {
		lines = append(lines, SummaryLine{Label: "Filter", Value: "<none>"})
	}

	if plan.Config.Prune != "" {
		lines = append(lines, SummaryLine{Label: "Prune Options", Value: plan.Config.Prune})
	} else {
		lines = append(lines, SummaryLine{Label: "Prune Options", Value: "<none>"})
	}

	lines = append(lines,
		SummaryLine{Label: "Log Retention", Value: fmt.Sprintf("%d", plan.Config.LogRetentionDays)},
		SummaryLine{Label: "Dry Run", Value: fmt.Sprintf("%t", plan.Request.DryRun)},
		SummaryLine{Label: "Force Prune", Value: fmt.Sprintf("%t", plan.Request.ForcePrune)},
		SummaryLine{Label: "Fix Perms", Value: fmt.Sprintf("%t", plan.Request.FixPerms)},
		SummaryLine{Label: "Prune Max %", Value: fmt.Sprintf("%d", plan.Config.SafePruneMaxDeletePercent)},
		SummaryLine{Label: "Prune Max Count", Value: fmt.Sprintf("%d", plan.Config.SafePruneMaxDeleteCount)},
		SummaryLine{Label: "Prune Min Total Revs", Value: fmt.Sprintf("%d", plan.Config.SafePruneMinTotalForPercent)},
	)

	if plan.Request.FixPerms {
		lines = append(lines,
			SummaryLine{Label: "Local Owner", Value: plan.Config.LocalOwner},
			SummaryLine{Label: "Local Group", Value: plan.Config.LocalGroup},
		)
	}

	if plan.Request.RemoteMode && plan.Secrets != nil {
		lines = append(lines,
			SummaryLine{Label: "Secrets Dir", Value: plan.SecretsDir},
			SummaryLine{Label: "Secrets File", Value: plan.SecretsFile},
			SummaryLine{Label: "STORJ S3 ID", Value: plan.Secrets.MaskedID()},
			SummaryLine{Label: "STORJ S3 Secret", Value: plan.Secrets.MaskedSecret()},
		)
	}

	return lines
}
