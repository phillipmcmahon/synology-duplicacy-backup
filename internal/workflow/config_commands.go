package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

var newConfigCommandRunner = func() execpkg.Runner {
	runner := execpkg.NewCommandRunner(nil, false)
	runner.SetDebugCommands(false)
	return runner
}

func HandleConfigCommand(req *Request, meta Metadata, rt Runtime) (string, error) {
	planner := NewPlanner(meta, rt, nil, newConfigCommandRunner())
	configReq := NewConfigRequest(req)

	switch configReq.Command {
	case "validate":
		return handleConfigValidate(&configReq, planner)
	case "explain":
		return handleConfigExplain(&configReq, planner)
	case "paths":
		return handleConfigPaths(&configReq, meta, planner), nil
	default:
		return "", NewRequestError("unsupported config command %q", configReq.Command)
	}
}

type ConfigCommandError struct {
	Message string
	Output  string
}

func (e *ConfigCommandError) Error() string {
	return e.Message
}

func ConfigCommandOutput(err error) string {
	var configErr *ConfigCommandError
	if errors.As(err, &configErr) {
		return configErr.Output
	}
	return ""
}

func handleConfigValidate(req *ConfigRequest, planner *Planner) (string, error) {
	plan := planner.derivePlan(req.PlanRequest())
	resolved := []SummaryLine{
		{Label: "Label", Value: req.Label},
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Config File", Value: plan.Paths.ConfigFile},
	}

	cfg, err := planner.loadConfigForValidation(plan)
	if err != nil {
		return "", err
	}
	resolved[2].Value = plan.Paths.ConfigFile
	plan.Config.Target = cfg.Target
	plan.Config.Location = cfg.Location
	plan.Paths.SnapshotSource = cfg.SourcePath
	plan.Paths.RepositoryPath = cfg.SourcePath
	plan.Paths.BackupTarget = cfg.Storage
	collector := newConfigValidationCollector([]SummaryLine{{Label: "Config", Value: "Valid"}})

	requiredErr := cfg.ValidateRequired(true, false)
	threadsErr := cfg.ValidateThreads()
	healthErr := cfg.ValidateThresholds()
	pruneStatus := "Not configured"
	var pruneErr error
	if cfg.Prune != "" {
		pruneStatus = "Valid"
		pruneErr = cfg.ValidatePrunePolicy()
	}

	sourceAccessible, sourceStatus, sourceErr := validateConfigSourcePathAccess(plan.Paths.SnapshotSource)
	btrfsStatus := "Not checked"
	var btrfsErr error
	if sourceAccessible {
		btrfsStatus = "Valid"
		btrfsErr = validateConfigSourceBtrfs(plan, planner.runner)
	}

	repoStatus := "Not checked"
	var repoErr error
	repoFailureMessage := ""
	repoHint := ""
	var sec *secrets.Secrets
	var secretsErr error
	storageSpec := duplicacy.NewStorageSpec(cfg.Storage)
	secretsNeeded := storageSpec.NeedsSecrets()
	secretsChecked := !secretsNeeded
	repositoryRequiresSudo := localRepositoryRequiresSudo(cfg, planner.rt)
	destinationStatus := presentation.ValueRequiresSudo
	var destinationErr error
	if !repositoryRequiresSudo {
		destinationStatus, destinationErr = validateConfigDestination(cfg)
	}
	targetSemanticsErr := cfg.ValidateTargetSemantics()
	if secretsNeeded {
		secretsChecked = true
		sec, secretsErr = planner.loadSecrets(plan)
		if secretsErr == nil {
			secretsErr = storageSpec.ValidateSecrets(sec)
		}
	}

	if sourceAccessible && destinationErr == nil {
		switch {
		case repositoryRequiresSudo:
			repoStatus = presentation.ValueRequiresSudo
			repoFailureMessage = presentation.LocalRepositoryRequiresSudoMessage("")
			repoHint = presentation.LocalRepositoryRequiresSudoMessage("")
		case secretsChecked && secretsErr == nil:
			repoStatus, repoHint, repoErr = validateConfigRepository(plan, cfg, planner.runner, sec)
		}
		if repoStatus == "Not initialized" && repoErr == nil {
			repoFailureMessage = "Repository is reachable but not initialized"
		}
	}

	collector.addStatus("Required Settings", "Valid", requiredErr)
	collector.addStatus("Threads", "Valid", threadsErr)
	if pruneErr != nil {
		collector.addStatus("Prune Policy", "Valid", pruneErr)
	} else {
		collector.addStatic("Prune Policy", pruneStatus)
	}
	collector.addStatus("Health Thresholds", "Valid", healthErr)
	collector.addStatus("Source Path Access", sourceStatus, sourceErr)
	if sourceAccessible {
		collector.addStatus("Btrfs Source", btrfsStatus, btrfsErr)
	} else {
		collector.addUnchecked("Btrfs Source")
	}
	if destinationErr == nil {
		collector.addStatic("Storage Access", destinationStatus)
	} else {
		collector.addStatus("Storage Access", destinationStatus, destinationErr)
	}
	switch {
	case repoErr != nil:
		collector.addStatus("Repository Access", repoStatus, repoErr)
	case repoFailureMessage != "":
		collector.addFailure("Repository Access", repoStatus, repoFailureMessage, repoHint)
	case repoStatus != "Not checked":
		collector.addStatic("Repository Access", repoStatus)
	default:
		collector.addUnchecked("Repository Access")
	}
	collector.addStatus("Target Settings", "Valid", targetSemanticsErr)
	if !secretsNeeded {
		collector.addStatic("Secrets", "Not required")
	} else if secretsChecked {
		collector.addStatus("Secrets", "Valid", secretsErr)
	} else {
		collector.addUnchecked("Secrets")
	}

	if collector.failed() {
		title := fmt.Sprintf("Config validation failed for %s/%s", req.Label, plan.TargetName())
		output := formatConfigValidationOutput(title, resolved, collector.lines, "Failed")
		message := title
		if hint := collector.failureHint(); hint != "" {
			message = fmt.Sprintf("%s; %s", message, hint)
		}
		return "", &ConfigCommandError{
			Message: message,
			Output:  output,
		}
	}

	return formatConfigValidationOutput(fmt.Sprintf("Config validation succeeded for %s/%s", req.Label, plan.TargetName()), resolved, collector.lines, "Passed"), nil
}

func handleConfigExplain(req *ConfigRequest, planner *Planner) (string, error) {
	plan := planner.derivePlan(req.PlanRequest())
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return "", err
	}
	plan.Config.Target = cfg.Target
	plan.Config.Location = cfg.Location
	plan.Display.ModeDisplay = modeDisplay(plan.TargetName())
	plan.Paths.SnapshotSource = cfg.SourcePath
	plan.Paths.BackupTarget = cfg.Storage
	plan.Config.Threads = cfg.Threads
	plan.Config.Filter = cfg.Filter
	plan.Config.PruneOptions = cfg.Prune
	plan.Config.LocalOwner = cfg.LocalOwner
	plan.Config.LocalGroup = cfg.LocalGroup

	lines := []SummaryLine{
		{Label: "Label", Value: req.Label},
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Location", Value: cfg.Location},
		{Label: "Config File", Value: plan.Paths.ConfigFile},
		{Label: "Source Path", Value: plan.Paths.SnapshotSource},
	}
	lines = append(lines, SummaryLine{Label: "Storage", Value: plan.Paths.BackupTarget})

	if cfg.Threads > 0 {
		lines = append(lines, SummaryLine{Label: "Threads", Value: fmt.Sprintf("%d", cfg.Threads)})
	}
	if cfg.Filter != "" {
		lines = append(lines, SummaryLine{Label: "Filter", Value: cfg.Filter})
	}
	if cfg.Prune != "" {
		lines = append(lines, SummaryLine{Label: "Prune Policy", Value: cfg.Prune})
	}
	if cfg.RestoreWorkspaceRoot != "" {
		lines = append(lines, SummaryLine{Label: "Restore Workspace Root", Value: cfg.RestoreWorkspaceRoot})
	}
	if cfg.RestoreWorkspaceTemplate != "" {
		lines = append(lines, SummaryLine{Label: "Restore Workspace Template", Value: cfg.RestoreWorkspaceTemplate})
	}

	if duplicacy.NewStorageSpec(cfg.Storage).NeedsSecrets() {
		lines = append(lines,
			SummaryLine{Label: "Secrets File", Value: plan.Paths.SecretsFile},
		)
	}
	if cfg.AllowLocalAccounts || cfg.LocalOwner != "" || cfg.LocalGroup != "" {
		lines = append(lines,
			SummaryLine{Label: "Allow Local Accounts", Value: fmt.Sprintf("%t", cfg.AllowLocalAccounts)},
			SummaryLine{Label: "Local Owner", Value: cfg.LocalOwner},
			SummaryLine{Label: "Local Group", Value: cfg.LocalGroup},
		)
	}

	return formatConfigOutput(fmt.Sprintf("Config explanation for %s/%s", req.Label, plan.TargetName()), lines), nil
}

func handleConfigPaths(req *ConfigRequest, meta Metadata, planner *Planner) string {
	plan := planner.derivePlan(req.PlanRequest())
	if cfg, err := planner.loadConfig(plan); err == nil {
		plan.Config.Target = cfg.Target
		plan.Config.Location = cfg.Location
		plan.Display.ModeDisplay = modeDisplay(plan.TargetName())
		plan.Paths.SnapshotSource = cfg.SourcePath
		plan.Paths.BackupTarget = cfg.Storage
	}
	lines := []SummaryLine{
		{Label: "Label", Value: req.Label},
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Location", Value: plan.Config.Location},
		{Label: "Config Dir", Value: plan.Paths.ConfigDir},
		{Label: "Config File", Value: plan.Paths.ConfigFile},
		{Label: "Source Path", Value: plan.Paths.SnapshotSource},
		{Label: "Log Dir", Value: meta.LogDir},
	}
	if duplicacy.NewStorageSpec(plan.Paths.BackupTarget).NeedsSecrets() {
		lines = append(lines,
			SummaryLine{Label: "Secrets Dir", Value: plan.Paths.SecretsDir},
			SummaryLine{Label: "Secrets File", Value: plan.Paths.SecretsFile},
		)
	}

	return formatConfigOutput(fmt.Sprintf("Resolved paths for %s", req.Label), lines)
}

func formatConfigOutput(title string, lines []SummaryLine) string {
	return presentation.FormatLines(title, lines)
}

func formatConfigValidationOutput(title string, resolved, validation []SummaryLine, result string) string {
	enableColour := logger.ColourEnabled(os.Stdout)
	return presentation.FormatValidationReport(title, resolved, validation, result, enableColour)
}

func colourizeConfigValidationValue(value string, enableColour bool) string {
	return presentation.ColourizeValidationValue(value, enableColour)
}

type configValidationCollector struct {
	lines  []SummaryLine
	errors []string
	hints  []string
}

func newConfigValidationCollector(lines []SummaryLine) *configValidationCollector {
	return &configValidationCollector{lines: lines}
}

func (c *configValidationCollector) addStatic(label, value string) {
	c.lines = append(c.lines, SummaryLine{Label: label, Value: value})
}

func (c *configValidationCollector) addUnchecked(label string) {
	c.addStatic(label, "Not checked")
}

func (c *configValidationCollector) addStatus(label, okValue string, err error) {
	if err != nil {
		msg := OperatorMessage(err)
		c.lines = append(c.lines, SummaryLine{Label: label, Value: fmt.Sprintf("Invalid (%s)", msg)})
		c.errors = append(c.errors, msg)
		return
	}
	c.lines = append(c.lines, SummaryLine{Label: label, Value: okValue})
}

func (c *configValidationCollector) addFailure(label, value, message, hint string) {
	c.lines = append(c.lines, SummaryLine{Label: label, Value: value})
	if message != "" {
		c.errors = append(c.errors, message)
	}
	if hint != "" {
		c.hints = append(c.hints, hint)
	}
}

func (c *configValidationCollector) failed() bool {
	return len(c.errors) > 0
}

func (c *configValidationCollector) failureHint() string {
	if len(c.hints) == 0 {
		return ""
	}
	return c.hints[0]
}

func validateConfigSourcePathAccess(sourcePath string) (bool, string, error) {
	if sourcePath == "" {
		return false, "", configPathError("source_path must not be empty")
	}
	if !filepath.IsAbs(sourcePath) {
		return false, "", configPathError(fmt.Sprintf("source_path must be an absolute path (was %q)", sourcePath))
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", configPathError(fmt.Sprintf("source_path does not exist: %s", sourcePath))
		}
		return false, "", configPathError(fmt.Sprintf("source_path is not accessible: %v", err))
	}
	if !info.IsDir() {
		return false, "", configPathError(fmt.Sprintf("source_path must be a directory or subvolume: %s", sourcePath))
	}
	return true, "Present", nil
}

func validateConfigSourceBtrfs(plan *Plan, runner execpkg.Runner) error {
	if err := btrfs.CheckVolume(runner, plan.Paths.SnapshotSource, false); err != nil {
		return err
	}
	return nil
}

func validateConfigDestination(cfg *config.Config) (string, error) {
	status, err := duplicacy.NewStorageSpec(cfg.Storage).ValidateForConfig()
	if err != nil {
		return "", configPathError(err.Error())
	}
	return status, nil
}

func validateConfigRepository(plan *Plan, cfg *config.Config, runner execpkg.Runner, sec *secrets.Secrets) (string, string, error) {
	dup, err := prepareConfigValidationProbe(plan, runner, sec)
	if err != nil {
		return "", "", err
	}
	defer dup.Cleanup()

	state, _, err := dup.ProbeRepository()
	switch state {
	case duplicacy.RepositoryAccessible:
		return "Valid", "", nil
	case duplicacy.RepositoryUninitialized:
		return "Not initialized", "initialize the repository before running backups", nil
	default:
		return "", "", err
	}
}

func prepareConfigValidationProbe(plan *Plan, runner execpkg.Runner, sec *secrets.Secrets) (*duplicacy.Setup, error) {
	dup := duplicacy.NewSetup(plan.Paths.WorkRoot, plan.Paths.RepositoryPath, plan.Paths.BackupTarget, false, runner)
	if err := dup.CreateDirs(); err != nil {
		return nil, err
	}
	if err := dup.WritePreferences(sec); err != nil {
		_ = dup.Cleanup()
		return nil, err
	}
	if err := dup.SetPermissions(); err != nil {
		_ = dup.Cleanup()
		return nil, err
	}
	return dup, nil
}

func configPathError(message string) error {
	return NewMessageError("%s", message)
}
