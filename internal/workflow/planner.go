package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
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

func (p *Planner) Build(req *Request) (*Plan, error) {
	if err := p.validateEnvironment(req); err != nil {
		return nil, err
	}

	plan := p.derivePlan(req)

	cfg, err := p.loadConfig(plan)
	if err != nil {
		return nil, err
	}
	plan.BackupTarget = JoinDestination(cfg.Destination, plan.BackupLabel)
	plan.OperationMode = OperationMode(req)
	plan.Threads = cfg.Threads
	plan.Filter = cfg.Filter
	plan.FilterLines = splitNonEmptyLines(cfg.Filter)
	plan.OwnerGroup = fmt.Sprintf("%s:%s", cfg.LocalOwner, cfg.LocalGroup)
	plan.PruneOptions = cfg.Prune
	plan.PruneArgs = append([]string(nil), cfg.PruneArgs...)
	plan.PruneArgsDisplay = strings.Join(cfg.PruneArgs, " ")
	plan.LocalOwner = cfg.LocalOwner
	plan.LocalGroup = cfg.LocalGroup
	plan.LogRetentionDays = cfg.LogRetentionDays
	plan.SafePruneMaxDeletePercent = cfg.SafePruneMaxDeletePercent
	plan.SafePruneMaxDeleteCount = cfg.SafePruneMaxDeleteCount
	plan.SafePruneMinTotalForPercent = cfg.SafePruneMinTotalForPercent

	if req.RemoteMode {
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

func (p *Planner) validateEnvironment(req *Request) error {
	if p.rt.Geteuid() != 0 {
		return fmt.Errorf("Must be run as root.")
	}
	if req.DoBackup || req.DoPrune {
		if _, err := p.rt.LookPath("duplicacy"); err != nil {
			return fmt.Errorf("Required command 'duplicacy' not found")
		}
	}
	if req.DoBackup {
		if _, err := p.rt.LookPath("btrfs"); err != nil {
			return fmt.Errorf("Required command 'btrfs' not found (needed for backup snapshots)")
		}
	}
	return nil
}

func (p *Planner) derivePlan(req *Request) *Plan {
	runTimestamp := p.rt.Now().Format("20060102-150405")
	backupLabel := req.Source
	workRoot := filepath.Join(
		p.rt.TempDir(),
		fmt.Sprintf("%s-%s-%s-%d", p.meta.ScriptName, backupLabel, runTimestamp, p.rt.Getpid()),
	)
	snapshotSource := filepath.Join(p.meta.RootVolume, backupLabel)
	snapshotTarget := filepath.Join(p.meta.RootVolume, fmt.Sprintf("%s-%s", backupLabel, runTimestamp))
	repositoryPath := snapshotSource
	if req.DoBackup {
		repositoryPath = snapshotTarget
	}
	configDir := ResolveDir(p.rt, req.ConfigDir, "DUPLICACY_BACKUP_CONFIG_DIR", ExecutableConfigDir(p.rt))
	secretsDir := ResolveDir(p.rt, req.SecretsDir, "DUPLICACY_BACKUP_SECRETS_DIR", config.DefaultSecretsDir)

	return &Plan{
		DoBackup:            req.DoBackup,
		DoPrune:             req.DoPrune,
		DeepPruneMode:       req.DeepPruneMode,
		FixPerms:            req.FixPerms,
		FixPermsOnly:        req.FixPermsOnly,
		ForcePrune:          req.ForcePrune,
		RemoteMode:          req.RemoteMode,
		DryRun:              req.DryRun,
		NeedsDuplicacySetup: req.DoBackup || req.DoPrune,
		NeedsSnapshot:       req.DoBackup,
		DefaultNotice:       req.DefaultNotice,
		ModeDisplay:         modeDisplay(req.RemoteMode),
		BackupLabel:         backupLabel,
		RunTimestamp:        runTimestamp,
		SnapshotSource:      snapshotSource,
		SnapshotTarget:      snapshotTarget,
		RepositoryPath:      repositoryPath,
		WorkRoot:            workRoot,
		DuplicacyRoot:       filepath.Join(workRoot, "duplicacy"),
		ConfigDir:           configDir,
		ConfigFile:          filepath.Join(configDir, fmt.Sprintf("%s-backup.conf", backupLabel)),
		SecretsDir:          secretsDir,
		SecretsFile:         secrets.GetSecretsFilePath(secretsDir, config.DefaultSecretsPrefix, backupLabel),
	}
}

func (p *Planner) loadConfig(plan *Plan) (*config.Config, error) {
	p.log.Info("Loading configuration from %s.", plan.ConfigFile)

	if _, err := os.Stat(plan.ConfigFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("Configuration file not found: %s.", plan.ConfigFile)
	}

	cfg := config.NewDefaults()
	targetSection := "local"
	if plan.RemoteMode {
		targetSection = "remote"
	}

	values, err := config.ParseFile(plan.ConfigFile, targetSection)
	if err != nil {
		return nil, err
	}
	if err := cfg.Apply(values); err != nil {
		return nil, err
	}

	p.log.Info("Configuration parsed for [common] + [%s].", targetSection)

	if err := cfg.ValidateRequired(plan.DoBackup, plan.DoPrune); err != nil {
		return nil, err
	}
	if err := cfg.ValidateThresholds(); err != nil {
		return nil, err
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
		if err := btrfs.CheckVolume(p.runner, p.meta.RootVolume, plan.DryRun); err != nil {
			return nil, err
		}
		p.log.Info("Verified '%s' is on a btrfs filesystem.", p.meta.RootVolume)

		if err := btrfs.CheckVolume(p.runner, plan.SnapshotSource, plan.DryRun); err != nil {
			return nil, err
		}
		p.log.Info("Verified '%s' is on a btrfs filesystem.", plan.SnapshotSource)
	}

	p.log.Info("Configuration loaded successfully.")
	return cfg, nil
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
	plan.DeepPruneCommand = "duplicacy prune -exhaustive -exclusive"
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

func modeDisplay(remote bool) string {
	if remote {
		return "REMOTE"
	}
	return "LOCAL"
}

func (p *Planner) loadSecrets(plan *Plan) (*secrets.Secrets, error) {
	p.log.Info("Loading secrets from %s.", plan.SecretsFile)

	sec, err := secrets.LoadSecretsFile(plan.SecretsFile)
	if err != nil {
		return nil, err
	}
	if err := sec.Validate(); err != nil {
		return nil, err
	}
	p.log.Info("Secrets loaded and validated.")
	return sec, nil
}
