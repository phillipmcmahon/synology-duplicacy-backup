package workflow

import (
	"fmt"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
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
	lines := []SummaryLine{
		{Label: "Config File", Value: planner.derivePlan(req).ConfigFile},
	}

	localPlan := planner.derivePlan(configValidationRequest(req, false))
	if _, err := planner.loadConfig(localPlan); err != nil {
		return "", err
	}
	lines = append(lines, SummaryLine{Label: "Local Config", Value: "Valid"})

	if req.RemoteMode {
		if err := validateRemoteConfig(planner, req); err != nil {
			return "", err
		}
		lines = append(lines,
			SummaryLine{Label: "Remote Config", Value: "Valid"},
			SummaryLine{Label: "Remote Secrets", Value: "Valid"},
		)
	} else {
		raw, err := config.ParseFile(localPlan.ConfigFile)
		if err != nil {
			return "", err
		}
		if raw.Remote != nil {
			if err := validateRemoteConfig(planner, req); err != nil {
				return "", err
			}
			lines = append(lines,
				SummaryLine{Label: "Remote Config", Value: "Valid"},
				SummaryLine{Label: "Remote Secrets", Value: "Valid"},
			)
		} else {
			lines = append(lines, SummaryLine{Label: "Remote Config", Value: "Not configured"})
		}
	}

	return formatConfigOutput(fmt.Sprintf("Config validation succeeded for %s", req.Source), lines), nil
}

func handleConfigExplain(req *Request, planner *Planner) (string, error) {
	planReq := configValidationRequest(req, req.RemoteMode)
	plan := planner.derivePlan(planReq)
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return "", err
	}
	plan.BackupTarget = JoinDestination(cfg.Destination, plan.BackupLabel)
	plan.Threads = cfg.Threads
	plan.PruneOptions = cfg.Prune
	plan.LocalOwner = cfg.LocalOwner
	plan.LocalGroup = cfg.LocalGroup

	lines := []SummaryLine{
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

	if req.RemoteMode {
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
			SummaryLine{Label: "Local Owner", Value: cfg.LocalOwner},
			SummaryLine{Label: "Local Group", Value: cfg.LocalGroup},
		)
	}

	return formatConfigOutput(fmt.Sprintf("Config explanation for %s", req.Source), lines), nil
}

func handleConfigPaths(req *Request, meta Metadata, planner *Planner) string {
	plan := planner.derivePlan(req)
	lines := []SummaryLine{
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

func validateRemoteConfig(planner *Planner, req *Request) error {
	remotePlan := planner.derivePlan(configValidationRequest(req, true))
	if _, err := planner.loadConfig(remotePlan); err != nil {
		return err
	}
	_, err := planner.loadSecrets(remotePlan)
	return err
}

func configValidationRequest(req *Request, remote bool) *Request {
	return &Request{
		Source:         req.Source,
		ConfigDir:      req.ConfigDir,
		SecretsDir:     req.SecretsDir,
		RemoteMode:     remote,
		DoBackup:       true,
		DoPrune:        true,
		DoCleanupStore: false,
		FixPerms:       !remote,
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
