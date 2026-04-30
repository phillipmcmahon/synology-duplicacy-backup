package workflow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

type Planner struct {
	meta   Metadata
	rt     Env
	log    *logger.Logger
	runner execpkg.Runner
}

func NewPlanner(meta Metadata, rt Env, log *logger.Logger, runner execpkg.Runner) *Planner {
	return &Planner{meta: meta, rt: rt, log: log, runner: runner}
}

// NewConfigPlanner creates a planner for config-only command paths. Callers
// must only use methods that derive plans or load configuration; runtime probes
// and execution paths require NewPlanner with a logger and command runner.
func NewConfigPlanner(meta Metadata, rt Env) *Planner {
	return NewPlanner(meta, rt, nil, nil)
}

func (p *Planner) Build(req *RuntimeRequest) (*Plan, error) {
	if err := p.validateEnvironment(req); err != nil {
		return nil, err
	}

	plan := p.deriveRuntimePlan(req)

	cfg, err := p.loadConfig(plan)
	if err != nil {
		return nil, err
	}
	plan.ApplyConfig(cfg, p.rt)
	if err := p.validateRepositoryMutationPrivilege(req, cfg); err != nil {
		return nil, err
	}
	if err := p.validateBackupFilesystem(plan); err != nil {
		return nil, err
	}

	if duplicacy.NewStorageSpec(cfg.Storage).NeedsSecrets() {
		sec, err := p.loadSecrets(plan)
		if err != nil {
			return nil, err
		}
		plan.Secrets = sec
	}

	plan.Summary = SummaryLines(plan)

	return plan, nil
}

func (p *Planner) FailureContext(req *RuntimeRequest) *Plan {
	if req == nil {
		return nil
	}

	plan := p.deriveRuntimePlan(req)
	if _, err := p.loadConfigForValidation(plan); err == nil {
		return plan
	}
	if plan.Config.Location != "" {
		return plan
	}
	return nil
}

func (p *Planner) validateEnvironment(req *RuntimeRequest) error {
	if p.rt.Geteuid() != 0 {
		switch {
		case req.DoBackup():
			return fmt.Errorf("backup must be run as root because it creates btrfs snapshots and reads the full source tree")
		}
	}
	if req.DoBackup() || req.DoPrune() || req.DoCleanupStore() {
		if _, err := p.rt.LookPath("duplicacy"); err != nil {
			return fmt.Errorf("required command 'duplicacy' not found")
		}
	}
	if req.DoBackup() {
		if _, err := p.rt.LookPath("btrfs"); err != nil {
			return fmt.Errorf("required command 'btrfs' not found (needed for backup snapshots)")
		}
	}
	return nil
}

func (p *Planner) validateRepositoryMutationPrivilege(req *RuntimeRequest, cfg *config.Config) error {
	if req == nil || cfg == nil || p.rt.Geteuid() == 0 {
		return nil
	}
	if !cfg.UsesRootProtectedLocalRepository() {
		return nil
	}

	command := ""
	switch {
	case req.DoPrune():
		command = "prune"
		if req.DryRun {
			command = "prune --dry-run"
		}
	case req.DoCleanupStore() && !req.DryRun:
		command = "cleanup-storage"
	default:
		return nil
	}
	return fmt.Errorf("%s", presentation.LocalRepositoryRequiresSudoMessage(command))
}

func (p *Planner) derivePlan(req ConfigPlanRequest) *Plan {
	return p.derivePlanFromInput(planDerivationInput{
		label:      req.Label,
		target:     req.Target(),
		configDir:  req.ConfigDir,
		secretsDir: req.SecretsDir,
	})
}

// DeriveConfigPlan exposes the config-only planning seam used by command
// subsystems that live outside internal/workflow.
func (p *Planner) DeriveConfigPlan(req ConfigPlanRequest) *Plan {
	return p.derivePlan(req)
}

func (p *Planner) deriveRuntimePlan(req *RuntimeRequest) *Plan {
	return p.derivePlanFromInput(planDerivationInput{
		label:               req.Label,
		target:              req.Target(),
		configDir:           req.ConfigDir,
		secretsDir:          req.SecretsDir,
		doBackup:            req.DoBackup(),
		doPrune:             req.DoPrune(),
		doCleanupStore:      req.DoCleanupStore(),
		forcePrune:          req.ForcePrune,
		dryRun:              req.DryRun,
		verbose:             req.Verbose,
		jsonSummary:         req.JSONSummary,
		defaultNotice:       req.DefaultNotice,
		operationMode:       OperationMode(req),
		needsDuplicacySetup: req.DoBackup() || req.DoPrune() || req.DoCleanupStore(),
		needsSnapshot:       req.DoBackup(),
	})
}

type planDerivationInput struct {
	label               string
	target              string
	configDir           string
	secretsDir          string
	doBackup            bool
	doPrune             bool
	doCleanupStore      bool
	forcePrune          bool
	dryRun              bool
	verbose             bool
	jsonSummary         bool
	defaultNotice       string
	operationMode       string
	needsDuplicacySetup bool
	needsSnapshot       bool
}

func (p *Planner) derivePlanFromInput(input planDerivationInput) *Plan {
	runTimestamp := p.rt.Now().Format("20060102-150405")
	backupLabel := input.label
	target := input.target
	workRoot := filepath.Join(
		p.rt.TempDir(),
		fmt.Sprintf("%s-%s-%s-%d", p.meta.ScriptName, backupLabel, runTimestamp, p.rt.Getpid()),
	)
	snapshotSource := filepath.Join(p.meta.RootVolume, backupLabel)
	snapshotTarget := filepath.Join(p.meta.RootVolume, fmt.Sprintf("%s-%s-%s-%d", backupLabel, target, runTimestamp, p.rt.Getpid()))
	repositoryPath := snapshotSource
	if input.doBackup {
		repositoryPath = snapshotTarget
	}
	configDir := ResolveDir(p.rt, input.configDir, "DUPLICACY_BACKUP_CONFIG_DIR", EffectiveConfigDir(p.rt))
	secretsDir := ResolveDir(p.rt, input.secretsDir, "DUPLICACY_BACKUP_SECRETS_DIR", EffectiveSecretsDir(p.rt))

	return &Plan{
		Request: PlanRequest{
			DoBackup:            input.doBackup,
			DoPrune:             input.doPrune,
			DoCleanupStore:      input.doCleanupStore,
			ForcePrune:          input.forcePrune,
			DryRun:              input.dryRun,
			Verbose:             input.verbose,
			JSONSummary:         input.jsonSummary,
			NeedsDuplicacySetup: input.needsDuplicacySetup,
			NeedsSnapshot:       input.needsSnapshot,
			DefaultNotice:       input.defaultNotice,
			OperationMode:       input.operationMode,
		},
		Config: PlanConfig{
			StorageName: target,
			BackupLabel: backupLabel,
		},
		Paths: PlanPaths{
			RunTimestamp:   runTimestamp,
			SnapshotSource: snapshotSource,
			SnapshotTarget: snapshotTarget,
			RepositoryPath: repositoryPath,
			WorkRoot:       workRoot,
			DuplicacyRoot:  filepath.Join(workRoot, "duplicacy"),
			ConfigDir:      configDir,
			ConfigFile:     filepath.Join(configDir, fmt.Sprintf("%s-backup.toml", backupLabel)),
			SecretsDir:     secretsDir,
			SecretsFile:    secrets.GetSecretsFilePath(secretsDir, backupLabel),
		},
	}
}

func (p *Planner) loadConfig(plan *Plan) (*config.Config, error) {
	return p.loadConfigWithOptions(plan, loadConfigOptions{validateThresholds: true, validateSemantics: true})
}

// LoadConfig resolves and validates the config for an already-derived plan.
func (p *Planner) LoadConfig(plan *Plan) (*config.Config, error) {
	return p.loadConfig(plan)
}

func (p *Planner) loadConfigForValidation(plan *Plan) (*config.Config, error) {
	return p.loadConfigWithOptions(plan, loadConfigOptions{})
}

type loadConfigOptions struct {
	validateThresholds bool
	validateSemantics  bool
}

func (p *Planner) loadConfigWithOptions(plan *Plan, opts loadConfigOptions) (*config.Config, error) {
	if _, err := os.Stat(plan.Paths.ConfigFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", plan.Paths.ConfigFile)
	}

	cfg := config.NewDefaults()
	raw, err := config.ParseFile(plan.Paths.ConfigFile)
	if err != nil {
		return nil, err
	}
	values, err := raw.ResolveValues(plan.TargetName(), plan.Paths.ConfigFile)
	if err != nil {
		return nil, err
	}
	if err := cfg.Apply(values); err != nil {
		return nil, err
	}
	cfg.Health = raw.ResolveHealth(cfg.StorageName)
	if cfg.Label == "" {
		return nil, apperrors.NewConfigError("label", fmt.Errorf("config file %s is missing required label value", plan.Paths.ConfigFile), "path", plan.Paths.ConfigFile)
	}
	if cfg.Label != plan.Config.BackupLabel {
		return nil, apperrors.NewConfigError("label", fmt.Errorf("config file %s defines label %q, expected %q", plan.Paths.ConfigFile, cfg.Label, plan.Config.BackupLabel), "path", plan.Paths.ConfigFile, "label", cfg.Label)
	}
	if cfg.StorageName == "" {
		cfg.StorageName = plan.TargetName()
	}
	plan.ApplyConfigIdentity(cfg)
	plan.Paths.SecretsFile = secrets.GetSecretsFilePath(plan.Paths.SecretsDir, plan.Config.BackupLabel)

	if err := cfg.ValidateRequired(plan.Request.DoBackup, plan.Request.DoPrune); err != nil {
		return nil, err
	}
	if opts.validateThresholds {
		if err := cfg.ValidateThresholds(); err != nil {
			return nil, err
		}
	}
	if opts.validateSemantics {
		if err := cfg.ValidateTargetSemantics(); err != nil {
			return nil, err
		}
	}
	cfg.BuildPruneArgs()
	if plan.Request.DoBackup {
		if err := cfg.ValidateThreads(); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func (p *Planner) validateBackupFilesystem(plan *Plan) error {
	if !plan.Request.DoBackup {
		return nil
	}

	if err := btrfs.CheckVolume(p.runner, p.meta.RootVolume, plan.Request.DryRun); err != nil {
		return err
	}
	if err := btrfs.CheckVolume(p.runner, plan.Paths.SnapshotSource, plan.Request.DryRun); err != nil {
		return err
	}

	return nil
}

func (p *Planner) loadSecrets(plan *Plan) (*secrets.Secrets, error) {
	sec, err := secrets.LoadSecretsFile(plan.Paths.SecretsFile, plan.Config.StorageName)
	if err != nil {
		return nil, err
	}
	if err := sec.Validate(); err != nil {
		return nil, err
	}
	return sec, nil
}

// LoadSecrets resolves and validates the secret file for an already-derived
// plan.
func (p *Planner) LoadSecrets(plan *Plan) (*secrets.Secrets, error) {
	return p.loadSecrets(plan)
}
