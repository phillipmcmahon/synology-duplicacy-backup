package workflow

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

func (h *HealthRunner) prepare(req *Request) (*config.Config, *Plan, *secrets.Secrets, error) {
	if h.rt.Geteuid() != 0 {
		return nil, nil, nil, NewMessageError("health commands must be run as root")
	}
	if _, err := h.rt.LookPath("duplicacy"); err != nil {
		return nil, nil, nil, NewMessageError("required command 'duplicacy' not found")
	}

	planner := NewPlanner(h.meta, h.rt, h.log, h.runner)
	plan := planner.derivePlan(req)

	cfgReq := configValidationRequest(req, req.Target())
	cfgPlan := planner.derivePlan(cfgReq)
	cfg, err := planner.loadConfig(cfgPlan)
	if err != nil {
		return nil, nil, nil, err
	}

	plan.Target = cfg.Target
	plan.Location = cfg.Location
	plan.ConfigFile = cfgPlan.ConfigFile
	plan.SecretsFile = cfgPlan.SecretsFile
	plan.BackupTarget = cfg.Storage
	plan.SnapshotSource = cfg.SourcePath
	plan.RepositoryPath = cfg.SourcePath
	plan.ModeDisplay = modeDisplay(plan.TargetName())
	plan.OperationMode = "Health " + presentation.Title(req.HealthCommand)
	plan.LocalOwner = cfg.LocalOwner
	plan.LocalGroup = cfg.LocalGroup
	plan.LogRetentionDays = cfg.LogRetentionDays
	plan.Filter = cfg.Filter
	plan.FilterLines = splitNonEmptyLines(cfg.Filter)
	plan.PruneOptions = cfg.Prune
	plan.Threads = cfg.Threads

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
	dup := duplicacy.NewSetup(plan.WorkRoot, plan.RepositoryPath, plan.BackupTarget, false, h.runner)
	if err := dup.CreateDirs(); err != nil {
		return nil, err
	}
	if err := dup.WritePreferences(sec); err != nil {
		return nil, err
	}
	if plan.Filter != "" {
		if err := dup.WriteFilters(plan.Filter); err != nil {
			return nil, err
		}
	}
	if err := dup.SetPermissions(); err != nil {
		return nil, err
	}
	return dup, nil
}
