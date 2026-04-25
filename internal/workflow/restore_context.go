package workflow

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

type restoreExecutionContext struct {
	plan       *Plan
	cfg        *configForRestore
	workspace  string
	mode       string
	secrets    *secrets.Secrets
	dup        *duplicacy.Setup
	cleanup    func()
	secretPath string
}

type configForRestore struct {
	Storage string
}

type restoreRunContext struct {
	plan      *Plan
	storage   string
	secrets   *secrets.Secrets
	workspace string
}

func newRestoreRunContext(req *RestoreRequest, meta Metadata, rt Runtime, deps RestoreDeps) (*restoreRunContext, error) {
	planner := NewConfigPlanner(meta, rt)
	plan := planner.derivePlan(req.ConfigRequest())
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return nil, err
	}
	plan.applyConfig(cfg, rt)

	storageSpec := duplicacy.NewStorageSpec(cfg.Storage)
	var sec *secrets.Secrets
	if storageSpec.NeedsSecrets() {
		sec, err = planner.loadSecrets(plan)
		if err != nil {
			return nil, err
		}
		if err := storageSpec.ValidateSecrets(sec); err != nil {
			return nil, err
		}
	}

	workspace, err := resolvedRestoreRunWorkspace(req, rt, plan, cfg.Storage, sec, deps)
	if err != nil {
		return nil, err
	}
	if err := validateRestoreWorkspace(workspace, plan.SnapshotSource); err != nil {
		return nil, err
	}

	return &restoreRunContext{
		plan:      plan,
		storage:   cfg.Storage,
		secrets:   sec,
		workspace: workspace,
	}, nil
}

func newRestoreExecutionContext(req *RestoreRequest, meta Metadata, rt Runtime, allowTemporary bool, deps RestoreDeps) (*restoreExecutionContext, error) {
	planner := NewConfigPlanner(meta, rt)
	plan := planner.derivePlan(req.ConfigRequest())
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return nil, err
	}
	plan.applyConfig(cfg, rt)

	storageSpec := duplicacy.NewStorageSpec(cfg.Storage)
	var sec *secrets.Secrets
	if storageSpec.NeedsSecrets() {
		sec, err = planner.loadSecrets(plan)
		if err != nil {
			return nil, err
		}
		if err := storageSpec.ValidateSecrets(sec); err != nil {
			return nil, err
		}
	}

	workspace, mode, cleanup, err := restoreWorkspaceForRead(req, plan, rt, allowTemporary, deps)
	if err != nil {
		return nil, err
	}
	if mode == "temporary" {
		if err := writeRestoreWorkspacePreferences(workspace, cfg.Storage, sec); err != nil {
			cleanup()
			return nil, err
		}
	}

	return &restoreExecutionContext{
		plan:       plan,
		cfg:        &configForRestore{Storage: cfg.Storage},
		workspace:  workspace,
		mode:       mode,
		secrets:    sec,
		dup:        duplicacy.NewWorkspaceSetup(workspace, cfg.Storage, false, deps.NewRunner()),
		cleanup:    cleanup,
		secretPath: plan.SecretsFile,
	}, nil
}
