package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

type Planner struct {
	meta   Metadata
	rt     Runtime
	log    *logger.Logger
	runner execpkg.Runner
}

func NewPlanner(meta Metadata, rt Runtime, log *logger.Logger, runner execpkg.Runner) *Planner {
	return &Planner{meta: meta, rt: rt, log: log, runner: runner}
}

// NewConfigPlanner creates a planner for config-only command paths. Callers
// must only use methods that derive plans or load configuration; runtime probes
// and execution paths require NewPlanner with a logger and command runner.
func NewConfigPlanner(meta Metadata, rt Runtime) *Planner {
	return NewPlanner(meta, rt, nil, nil)
}

func (p *Planner) Build(req *Request) (*Plan, error) {
	if err := p.validateEnvironment(req); err != nil {
		return nil, err
	}

	plan := p.deriveRuntimePlan(req)

	cfg, err := p.loadConfig(plan)
	if err != nil {
		return nil, err
	}
	plan.applyConfig(cfg, p.rt)
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

	p.populateCommands(plan)
	plan.Summary = SummaryLines(plan)

	return plan, nil
}

func (p *Planner) FailureContext(req *Request) *Plan {
	if req == nil {
		return nil
	}

	plan := p.deriveRuntimePlan(req)
	if _, err := p.loadConfigForValidation(plan); err == nil {
		return plan
	}
	if plan.Location != "" {
		return plan
	}
	return nil
}

func (p *Planner) validateEnvironment(req *Request) error {
	if p.rt.Geteuid() != 0 {
		return fmt.Errorf("must be run as root")
	}
	if req.DoBackup || req.DoPrune || req.DoCleanupStore {
		if _, err := p.rt.LookPath("duplicacy"); err != nil {
			return fmt.Errorf("required command 'duplicacy' not found")
		}
	}
	if req.DoBackup {
		if _, err := p.rt.LookPath("btrfs"); err != nil {
			return fmt.Errorf("required command 'btrfs' not found (needed for backup snapshots)")
		}
	}
	return nil
}

func (p *Planner) derivePlan(req ConfigPlanRequest) *Plan {
	return p.derivePlanFromInput(planDerivationInput{
		label:      req.Label,
		target:     req.Target(),
		configDir:  req.ConfigDir,
		secretsDir: req.SecretsDir,
	})
}

func (p *Planner) deriveRuntimePlan(req *Request) *Plan {
	return p.derivePlanFromInput(planDerivationInput{
		label:               req.Source,
		target:              req.Target(),
		configDir:           req.ConfigDir,
		secretsDir:          req.SecretsDir,
		doBackup:            req.DoBackup,
		doPrune:             req.DoPrune,
		doCleanupStore:      req.DoCleanupStore,
		fixPerms:            req.FixPerms,
		fixPermsOnly:        req.FixPermsOnly,
		forcePrune:          req.ForcePrune,
		dryRun:              req.DryRun,
		verbose:             req.Verbose,
		jsonSummary:         req.JSONSummary,
		defaultNotice:       req.DefaultNotice,
		operationMode:       OperationMode(req),
		needsDuplicacySetup: req.DoBackup || req.DoPrune || req.DoCleanupStore,
		needsSnapshot:       req.DoBackup,
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
	fixPerms            bool
	fixPermsOnly        bool
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
	configDir := ResolveDir(p.rt, input.configDir, "DUPLICACY_BACKUP_CONFIG_DIR", ExecutableConfigDir(p.rt))
	secretsDir := ResolveDir(p.rt, input.secretsDir, "DUPLICACY_BACKUP_SECRETS_DIR", config.DefaultSecretsDir)

	return &Plan{
		DoBackup:            input.doBackup,
		DoPrune:             input.doPrune,
		DoCleanupStore:      input.doCleanupStore,
		FixPerms:            input.fixPerms,
		FixPermsOnly:        input.fixPermsOnly,
		ForcePrune:          input.forcePrune,
		DryRun:              input.dryRun,
		Verbose:             input.verbose,
		JSONSummary:         input.jsonSummary,
		NeedsDuplicacySetup: input.needsDuplicacySetup,
		NeedsSnapshot:       input.needsSnapshot,
		DefaultNotice:       input.defaultNotice,
		OperationMode:       input.operationMode,
		ModeDisplay:         modeDisplay(target),
		Target:              target,
		BackupLabel:         backupLabel,
		RunTimestamp:        runTimestamp,
		SnapshotSource:      snapshotSource,
		SnapshotTarget:      snapshotTarget,
		RepositoryPath:      repositoryPath,
		WorkRoot:            workRoot,
		DuplicacyRoot:       filepath.Join(workRoot, "duplicacy"),
		ConfigDir:           configDir,
		ConfigFile:          filepath.Join(configDir, fmt.Sprintf("%s-backup.toml", backupLabel)),
		SecretsDir:          secretsDir,
		SecretsFile:         secrets.GetSecretsFilePath(secretsDir, backupLabel),
	}
}

func (p *Planner) loadConfig(plan *Plan) (*config.Config, error) {
	return p.loadConfigWithOptions(plan, loadConfigOptions{validateThresholds: true, validateSemantics: true})
}

func (p *Planner) loadConfigForValidation(plan *Plan) (*config.Config, error) {
	return p.loadConfigWithOptions(plan, loadConfigOptions{})
}

type loadConfigOptions struct {
	validateThresholds bool
	validateSemantics  bool
}

func (p *Planner) loadConfigWithOptions(plan *Plan, opts loadConfigOptions) (*config.Config, error) {
	if _, err := os.Stat(plan.ConfigFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", plan.ConfigFile)
	}

	cfg := config.NewDefaults()
	raw, err := config.ParseFile(plan.ConfigFile)
	if err != nil {
		return nil, err
	}
	values, err := raw.ResolveValues(plan.TargetName(), plan.ConfigFile)
	if err != nil {
		return nil, err
	}
	if err := cfg.Apply(values); err != nil {
		return nil, err
	}
	cfg.Health = raw.ResolveHealth(cfg.Target)
	if cfg.Label == "" {
		return nil, apperrors.NewConfigError("label", fmt.Errorf("config file %s is missing required label value", plan.ConfigFile), "path", plan.ConfigFile)
	}
	if cfg.Label != plan.BackupLabel {
		return nil, apperrors.NewConfigError("label", fmt.Errorf("config file %s defines label %q, expected %q", plan.ConfigFile, cfg.Label, plan.BackupLabel), "path", plan.ConfigFile, "label", cfg.Label)
	}
	if cfg.Target == "" {
		cfg.Target = plan.TargetName()
	}
	plan.applyConfigIdentity(cfg)
	plan.SecretsFile = secrets.GetSecretsFilePath(plan.SecretsDir, plan.BackupLabel)

	if err := cfg.ValidateRequired(plan.DoBackup, plan.DoPrune); err != nil {
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
		if plan.FixPerms && !duplicacy.NewStorageSpec(cfg.Storage).SupportsFixPerms() {
			return nil, apperrors.NewConfigError("fix-perms", fmt.Errorf("fix-perms is only supported for path-based Duplicacy storage targets"))
		}
	}
	if plan.FixPerms {
		if err := cfg.ValidateOwnerGroup(); err != nil {
			return nil, err
		}
	}
	cfg.BuildPruneArgs()
	if plan.DoBackup {
		if err := cfg.ValidateThreads(); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func (p *Plan) applyConfigIdentity(cfg *config.Config) {
	if p == nil || cfg == nil {
		return
	}
	p.Target = cfg.Target
	p.Location = cfg.Location
	p.Notify = cfg.Health.Notify
	p.ModeDisplay = modeDisplay(cfg.Target)
}

func (p *Plan) applyConfig(cfg *config.Config, rt Runtime) {
	if p == nil || cfg == nil {
		return
	}
	p.applyConfigIdentity(cfg)
	p.SnapshotSource = cfg.SourcePath
	p.SnapshotTarget = filepath.Join(rootVolumeForSource(cfg.SourcePath), fmt.Sprintf("%s-%s-%s-%d", p.BackupLabel, p.TargetName(), p.RunTimestamp, rt.Getpid()))
	p.RepositoryPath = cfg.SourcePath
	if p.DoBackup {
		p.RepositoryPath = p.SnapshotTarget
	}
	p.BackupTarget = cfg.Storage
	p.Threads = cfg.Threads
	p.Filter = cfg.Filter
	p.FilterLines = splitNonEmptyLines(cfg.Filter)
	p.OwnerGroup = fmt.Sprintf("%s:%s", cfg.LocalOwner, cfg.LocalGroup)
	p.PruneOptions = cfg.Prune
	p.PruneArgs = append([]string(nil), cfg.PruneArgs...)
	p.PruneArgsDisplay = strings.Join(cfg.PruneArgs, " ")
	p.LocalOwner = cfg.LocalOwner
	p.LocalGroup = cfg.LocalGroup
	p.LogRetentionDays = cfg.LogRetentionDays
	p.SafePruneMaxDeletePercent = cfg.SafePruneMaxDeletePercent
	p.SafePruneMaxDeleteCount = cfg.SafePruneMaxDeleteCount
	p.SafePruneMinTotalForPercent = cfg.SafePruneMinTotalForPercent
}

func (p *Planner) validateBackupFilesystem(plan *Plan) error {
	if !plan.DoBackup {
		return nil
	}

	if err := btrfs.CheckVolume(p.runner, p.meta.RootVolume, plan.DryRun); err != nil {
		return err
	}
	if err := btrfs.CheckVolume(p.runner, plan.SnapshotSource, plan.DryRun); err != nil {
		return err
	}

	return nil
}

func (p *Planner) populateCommands(plan *Plan) {
	plan.SnapshotCreateCommand = fmt.Sprintf("btrfs subvolume snapshot -r %s %s", plan.SnapshotSource, plan.SnapshotTarget)
	plan.SnapshotDeleteCommand = fmt.Sprintf("btrfs subvolume delete %s", plan.SnapshotTarget)
	plan.WorkDirCreateCommand = fmt.Sprintf("mkdir -p %s", filepath.Join(plan.DuplicacyRoot, ".duplicacy"))
	plan.PreferencesWriteCommand = fmt.Sprintf("write JSON preferences to %s", filepath.Join(plan.DuplicacyRoot, ".duplicacy", "preferences"))
	plan.FiltersWriteCommand = fmt.Sprintf("write filters to %s", filepath.Join(plan.DuplicacyRoot, ".duplicacy", "filters"))
	plan.WorkDirDirPermsCommand = fmt.Sprintf("find %s -type d -exec chmod 770 {} +", plan.DuplicacyRoot)
	plan.WorkDirFilePermsCommand = fmt.Sprintf("find %s -type f -exec chmod 660 {} +", plan.DuplicacyRoot)
	plan.BackupCommand = fmt.Sprintf("duplicacy backup -stats -threads %d", plan.Threads)
	plan.ValidateRepoCommand = "duplicacy list -files"
	plan.PrunePreviewCommand = strings.TrimSpace(fmt.Sprintf("duplicacy prune %s -dry-run", plan.PruneArgsDisplay))
	if plan.PrunePreviewCommand == "duplicacy prune  -dry-run" {
		plan.PrunePreviewCommand = "duplicacy prune -dry-run"
	}
	plan.PolicyPruneCommand = strings.TrimSpace(fmt.Sprintf("duplicacy prune %s", plan.PruneArgsDisplay))
	if plan.PolicyPruneCommand == "duplicacy prune" || plan.PolicyPruneCommand == "duplicacy prune " {
		plan.PolicyPruneCommand = "duplicacy prune"
	}
	plan.CleanupStorageCommand = "duplicacy prune -exhaustive -exclusive"
	plan.FixPermsChownCommand = fmt.Sprintf("chown -R %s %s", plan.OwnerGroup, plan.BackupTarget)
	plan.FixPermsDirPermsCommand = fmt.Sprintf("find %s -type d -exec chmod 770 {} +", plan.BackupTarget)
	plan.FixPermsFilePermsCommand = fmt.Sprintf("find %s -type f -exec chmod 660 {} +", plan.BackupTarget)
	plan.WorkDirRemoveCommand = fmt.Sprintf("rm -rf %s", plan.WorkRoot)
}

func splitNonEmptyLines(value string) []string {
	if value == "" {
		return nil
	}
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func modeDisplay(targetName string) string {
	if targetName != "" {
		return targetName
	}
	return "not supplied"
}

func rootVolumeForSource(sourcePath string) string {
	clean := filepath.Clean(sourcePath)
	if clean == "." || clean == "/" {
		return clean
	}
	if !filepath.IsAbs(clean) {
		return clean
	}
	trimmed := strings.TrimPrefix(clean, string(filepath.Separator))
	parts := strings.Split(trimmed, string(filepath.Separator))
	if len(parts) == 0 || parts[0] == "" {
		return string(filepath.Separator)
	}
	return string(filepath.Separator) + parts[0]
}

func (p *Planner) loadSecrets(plan *Plan) (*secrets.Secrets, error) {
	sec, err := secrets.LoadSecretsFile(plan.SecretsFile, plan.Target)
	if err != nil {
		return nil, err
	}
	if err := sec.Validate(); err != nil {
		return nil, err
	}
	return sec, nil
}
