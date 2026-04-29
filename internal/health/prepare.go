package health

import (
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/operator"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func (h *HealthRunner) prepare(req *HealthRequest) (*config.Config, *Plan, *secrets.Secrets, error) {
	if _, err := h.rt.LookPath("duplicacy"); err != nil {
		return nil, nil, nil, operator.NewMessageError("required command 'duplicacy' not found")
	}

	planner := workflow.NewPlanner(h.meta, h.rt, h.log, h.runner)
	plan := planner.DeriveConfigPlan(req.PlanRequest())

	cfgPlan := planner.DeriveConfigPlan(req.PlanRequest())
	cfg, err := planner.LoadConfig(cfgPlan)
	if err != nil {
		return nil, nil, nil, err
	}

	plan.Config.Target = cfg.Target
	plan.Config.Location = cfg.Location
	plan.Paths.ConfigFile = cfgPlan.Paths.ConfigFile
	plan.Paths.SecretsFile = cfgPlan.Paths.SecretsFile
	plan.Paths.BackupTarget = cfg.Storage
	plan.Paths.SnapshotSource = cfg.SourcePath
	plan.Paths.RepositoryPath = cfg.SourcePath
	plan.Request.OperationMode = "Health " + presentation.Title(req.Command)
	plan.Config.LogRetentionDays = cfg.LogRetentionDays
	plan.Config.Filter = cfg.Filter
	plan.Config.FilterLines = nonEmptyLines(cfg.Filter)
	plan.Config.PruneOptions = cfg.Prune
	plan.Config.Threads = cfg.Threads

	var sec *secrets.Secrets
	if duplicacy.NewStorageSpec(cfg.Storage).NeedsSecrets() {
		sec, err = planner.LoadSecrets(cfgPlan)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	plan.Secrets = sec
	return cfg, plan, sec, nil
}

func (h *HealthRunner) prepareDuplicacySetup(plan *Plan, sec *secrets.Secrets) (*duplicacy.Setup, error) {
	dup := duplicacy.NewSetup(plan.Paths.WorkRoot, plan.Paths.RepositoryPath, plan.Paths.BackupTarget, false, h.runner)
	if err := dup.CreateDirs(); err != nil {
		return nil, err
	}
	if err := dup.WritePreferences(sec); err != nil {
		return nil, err
	}
	if plan.Config.Filter != "" {
		if err := dup.WriteFilters(plan.Config.Filter); err != nil {
			return nil, err
		}
	}
	if err := dup.SetPermissions(); err != nil {
		return nil, err
	}
	return dup, nil
}

func nonEmptyLines(value string) []string {
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
