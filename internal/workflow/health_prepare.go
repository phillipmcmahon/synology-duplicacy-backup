package workflow

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

func (h *HealthRunner) prepare(req *HealthRequest) (*config.Config, *Plan, *secrets.Secrets, error) {
	if _, err := h.rt.LookPath("duplicacy"); err != nil {
		return nil, nil, nil, NewMessageError("required command 'duplicacy' not found")
	}

	planner := NewPlanner(h.meta, h.rt, h.log, h.runner)
	plan := planner.derivePlan(req.PlanRequest())

	cfgPlan := planner.derivePlan(req.PlanRequest())
	cfg, err := planner.loadConfig(cfgPlan)
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
	plan.Display.ModeDisplay = modeDisplay(plan.TargetName())
	plan.Request.OperationMode = "Health " + presentation.Title(req.Command)
	plan.Config.LocalOwner = cfg.LocalOwner
	plan.Config.LocalGroup = cfg.LocalGroup
	plan.Config.LogRetentionDays = cfg.LogRetentionDays
	plan.Config.Filter = cfg.Filter
	plan.Config.FilterLines = splitNonEmptyLines(cfg.Filter)
	plan.Config.PruneOptions = cfg.Prune
	plan.Config.Threads = cfg.Threads

	var sec *secrets.Secrets
	if duplicacy.NewStorageSpec(cfg.Storage).NeedsSecrets() {
		sec, err = planner.loadSecrets(cfgPlan)
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
