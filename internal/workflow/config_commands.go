package workflow

import (
	"fmt"
	"strings"
)

func HandleConfigCommand(req *Request, meta Metadata, rt Runtime) (string, error) {
	planner := NewPlanner(meta, rt, nil, nil)

	switch req.ConfigCommand {
	case "validate":
		return handleConfigValidate(req, planner)
	case "explain":
		return handleConfigExplain(req, planner)
	case "paths":
		return handleConfigPaths(req, meta, planner), nil
	default:
		return "", NewRequestError("unsupported config command %q", req.ConfigCommand)
	}
}

func handleConfigValidate(req *Request, planner *Planner) (string, error) {
	target := req.Target()
	planReq := configValidationRequest(req, target)
	plan := planner.derivePlan(planReq)
	lines := []SummaryLine{
		{Label: "Target", Value: target},
		{Label: "Config File", Value: plan.ConfigFile},
	}

	if _, err := planner.loadConfig(plan); err != nil {
		return "", err
	}
	lines[1].Value = plan.ConfigFile
	lines = append(lines, SummaryLine{Label: "Config", Value: "Valid"})

	if plan.TargetType == targetRemote {
		if _, err := planner.loadSecrets(plan); err != nil {
			return "", err
		}
		lines = append(lines, SummaryLine{Label: "Secrets", Value: "Valid"})
	}

	return formatConfigOutput(fmt.Sprintf("Config validation succeeded for %s/%s", req.Source, target), lines), nil
}

func handleConfigExplain(req *Request, planner *Planner) (string, error) {
	planReq := configValidationRequest(req, req.Target())
	plan := planner.derivePlan(planReq)
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return "", err
	}
	plan.Target = cfg.Target
	plan.TargetType = cfg.TargetType
	plan.ModeDisplay = modeDisplay(plan.TargetName(), plan.TargetType)
	plan.SnapshotSource = cfg.SourcePath
	plan.BackupTarget = JoinDestination(cfg.Destination, cfg.Repository)
	plan.Threads = cfg.Threads
	plan.PruneOptions = cfg.Prune
	plan.LocalOwner = cfg.LocalOwner
	plan.LocalGroup = cfg.LocalGroup

	lines := []SummaryLine{
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Mode", Value: plan.ModeDisplay},
		{Label: "Config File", Value: plan.ConfigFile},
		{Label: "Source", Value: plan.SnapshotSource},
		{Label: "Destination", Value: plan.BackupTarget},
	}

	if cfg.Threads > 0 {
		lines = append(lines, SummaryLine{Label: "Threads", Value: fmt.Sprintf("%d", cfg.Threads)})
	}
	if cfg.Prune != "" {
		lines = append(lines, SummaryLine{Label: "Prune Policy", Value: cfg.Prune})
	}

	if plan.TargetType == targetRemote {
		sec, err := planner.loadSecrets(plan)
		if err != nil {
			return "", err
		}
		lines = append(lines,
			SummaryLine{Label: "Secrets File", Value: plan.SecretsFile},
			SummaryLine{Label: "Remote Access Key", Value: sec.MaskedID()},
			SummaryLine{Label: "Remote Secret Key", Value: sec.MaskedSecret()},
		)
	} else {
		lines = append(lines,
			SummaryLine{Label: "Allow Local Accounts", Value: fmt.Sprintf("%t", cfg.AllowLocalAccounts)},
			SummaryLine{Label: "Local Owner", Value: cfg.LocalOwner},
			SummaryLine{Label: "Local Group", Value: cfg.LocalGroup},
		)
	}

	return formatConfigOutput(fmt.Sprintf("Config explanation for %s/%s", req.Source, plan.TargetName()), lines), nil
}

func handleConfigPaths(req *Request, meta Metadata, planner *Planner) string {
	plan := planner.derivePlan(req)
	if _, err := planner.loadConfig(plan); err == nil {
		// Use the resolved config path when the file exists.
	}
	lines := []SummaryLine{
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Mode", Value: plan.ModeDisplay},
		{Label: "Config Dir", Value: plan.ConfigDir},
		{Label: "Config File", Value: plan.ConfigFile},
		{Label: "Secrets Dir", Value: plan.SecretsDir},
		{Label: "Secrets File", Value: plan.SecretsFile},
		{Label: "Source Path", Value: plan.SnapshotSource},
		{Label: "Log Dir", Value: meta.LogDir},
	}

	return formatConfigOutput(fmt.Sprintf("Resolved paths for %s", req.Source), lines)
}

func configValidationRequest(req *Request, target string) *Request {
	return &Request{
		Source:          req.Source,
		ConfigDir:       req.ConfigDir,
		SecretsDir:      req.SecretsDir,
		RequestedTarget: target,
		RemoteMode:      target == targetRemote,
		DoBackup:        false,
		DoPrune:         false,
		DoCleanupStore:  false,
		FixPerms:        false,
	}
}

func formatConfigOutput(title string, lines []SummaryLine) string {
	var b strings.Builder
	b.WriteString(title)
	b.WriteByte('\n')
	for _, line := range lines {
		fmt.Fprintf(&b, "  %-20s : %s\n", line.Label, line.Value)
	}
	return b.String()
}
