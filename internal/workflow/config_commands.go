package workflow

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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

func handleConfigValidate(req *Request, planner *Planner) (string, error) {
	planReq := configValidationRequest(req, req.Target())
	plan := planner.derivePlan(planReq)
	resolved := []SummaryLine{
		{Label: "Label", Value: req.Source},
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Config File", Value: plan.ConfigFile},
	}

	cfg, err := planner.loadConfigForValidation(plan)
	if err != nil {
		return "", err
	}
	resolved[2].Value = plan.ConfigFile
	plan.Target = cfg.Target
	plan.StorageType = cfg.StorageType
	plan.Location = cfg.Location
	plan.SnapshotSource = cfg.SourcePath
	plan.RepositoryPath = cfg.SourcePath
	plan.BackupTarget = ResolveBackupTarget(cfg.StorageType, cfg.Destination, cfg.Storage, cfg.Repository)
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

	sourceAccessible, sourceStatus, sourceErr := validateConfigSourcePathAccess(plan.SnapshotSource)
	privilegedValidation := planner.rt.Geteuid() == 0
	privilegeStatus := "Limited"
	if privilegedValidation {
		privilegeStatus = "Full"
	}
	btrfsStatus := "Not checked"
	var btrfsErr error
	if sourceAccessible && privilegedValidation {
		btrfsStatus = "Valid"
		btrfsErr = validateConfigSourceBtrfs(plan, planner.runner)
	}

	destinationStatus, destinationErr := validateConfigDestination(cfg)

	repoStatus := "Not checked"
	var repoErr error
	repoFailureMessage := ""
	repoHint := ""
	var sec *secrets.Secrets
	var secretsErr error
	secretsNeeded := cfg.UsesDuplicacyStorage() && duplicacyStorageNeedsSecrets(cfg.Storage)
	secretsChecked := !cfg.UsesDuplicacyStorage() || !secretsNeeded
	targetSemanticsErr := cfg.ValidateTargetSemantics()
	if secretsNeeded && privilegedValidation {
		secretsChecked = true
		sec, secretsErr = planner.loadSecrets(plan)
		if secretsErr == nil {
			secretsErr = validateDuplicacyStorageSecrets(cfg.Storage, sec)
		}
	}

	if sourceAccessible && destinationErr == nil {
		switch {
		case cfg.UsesDuplicacyStorage() && secretsChecked && secretsErr == nil:
			repoStatus, repoHint, repoErr = validateConfigRepository(plan, cfg, planner.runner, sec)
		}
		if repoStatus == "Not initialized" && repoErr == nil {
			repoFailureMessage = "Repository is reachable but not initialized"
		}
	}

	collector.addStatus("Required Settings", "Valid", requiredErr)
	collector.addStatic("Privileges", privilegeStatus)
	collector.addStatus("Threads", "Valid", threadsErr)
	if pruneErr != nil {
		collector.addStatus("Prune Policy", "Valid", pruneErr)
	} else {
		collector.addStatic("Prune Policy", pruneStatus)
	}
	collector.addStatus("Health Thresholds", "Valid", healthErr)
	collector.addStatus("Source Path Access", sourceStatus, sourceErr)
	if sourceAccessible && privilegedValidation {
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
	if cfg.UsesDuplicacyStorage() {
		if !secretsNeeded {
			collector.addStatic("Secrets", "Not required")
		} else if secretsChecked {
			collector.addStatus("Secrets", "Valid", secretsErr)
		} else {
			collector.addUnchecked("Secrets")
		}
	}

	if collector.failed() {
		title := fmt.Sprintf("Config validation failed for %s/%s", req.Source, plan.TargetName())
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

	return formatConfigValidationOutput(fmt.Sprintf("Config validation succeeded for %s/%s", req.Source, plan.TargetName()), resolved, collector.lines, "Passed"), nil
}

func handleConfigExplain(req *Request, planner *Planner) (string, error) {
	planReq := configValidationRequest(req, req.Target())
	plan := planner.derivePlan(planReq)
	cfg, err := planner.loadConfig(plan)
	if err != nil {
		return "", err
	}
	plan.Target = cfg.Target
	plan.StorageType = cfg.StorageType
	plan.Location = cfg.Location
	plan.ModeDisplay = modeDisplay(plan.TargetName(), plan.StorageType)
	plan.SnapshotSource = cfg.SourcePath
	plan.BackupTarget = ResolveBackupTarget(cfg.StorageType, cfg.Destination, cfg.Storage, cfg.Repository)
	plan.Threads = cfg.Threads
	plan.Filter = cfg.Filter
	plan.PruneOptions = cfg.Prune
	plan.LocalOwner = cfg.LocalOwner
	plan.LocalGroup = cfg.LocalGroup

	lines := []SummaryLine{
		{Label: "Label", Value: req.Source},
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Location", Value: cfg.Location},
		{Label: "Config File", Value: plan.ConfigFile},
		{Label: "Source", Value: plan.SnapshotSource},
	}
	lines = append(lines, SummaryLine{Label: "Storage", Value: plan.BackupTarget})

	if cfg.Threads > 0 {
		lines = append(lines, SummaryLine{Label: "Threads", Value: fmt.Sprintf("%d", cfg.Threads)})
	}
	if cfg.Filter != "" {
		lines = append(lines, SummaryLine{Label: "Filter", Value: cfg.Filter})
	}
	if cfg.Prune != "" {
		lines = append(lines, SummaryLine{Label: "Prune Policy", Value: cfg.Prune})
	}

	if cfg.UsesDuplicacyStorage() && duplicacyStorageNeedsSecrets(cfg.Storage) {
		lines = append(lines,
			SummaryLine{Label: "Secrets File", Value: plan.SecretsFile},
		)
	}
	if cfg.AllowLocalAccounts || cfg.LocalOwner != "" || cfg.LocalGroup != "" {
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
	if cfg, err := planner.loadConfig(plan); err == nil {
		plan.Target = cfg.Target
		plan.StorageType = cfg.StorageType
		plan.Location = cfg.Location
		plan.ModeDisplay = modeDisplay(plan.TargetName(), plan.StorageType)
		plan.SnapshotSource = cfg.SourcePath
		plan.BackupTarget = ResolveBackupTarget(cfg.StorageType, cfg.Destination, cfg.Storage, cfg.Repository)
	}
	lines := []SummaryLine{
		{Label: "Label", Value: req.Source},
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Location", Value: plan.Location},
		{Label: "Config Dir", Value: plan.ConfigDir},
		{Label: "Config File", Value: plan.ConfigFile},
		{Label: "Source Path", Value: plan.SnapshotSource},
		{Label: "Log Dir", Value: meta.LogDir},
	}
	if plan.UsesDuplicacyStorage() && duplicacyStorageNeedsSecrets(plan.BackupTarget) {
		lines = append(lines,
			SummaryLine{Label: "Secrets Dir", Value: plan.SecretsDir},
			SummaryLine{Label: "Secrets File", Value: plan.SecretsFile},
		)
	}

	return formatConfigOutput(fmt.Sprintf("Resolved paths for %s", req.Source), lines)
}

func configValidationRequest(req *Request, target string) *Request {
	return &Request{
		Source:          req.Source,
		ConfigDir:       req.ConfigDir,
		SecretsDir:      req.SecretsDir,
		RequestedTarget: target,
		DoBackup:        false,
		DoPrune:         false,
		DoCleanupStore:  false,
		FixPerms:        false,
	}
}

func formatConfigOutput(title string, lines []SummaryLine) string {
	return presentation.FormatLines(title, lines)
}

func formatConfigValidationOutput(title string, resolved, validation []SummaryLine, result string) string {
	enableColour := logger.IsTerminal(os.Stdout)
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
	f, err := os.Open(sourcePath)
	if err != nil {
		return false, "", configPathError(fmt.Sprintf("source_path is not readable: %s", sourcePath))
	}
	_ = f.Close()
	return true, "Readable", nil
}

func validateConfigSourceBtrfs(plan *Plan, runner execpkg.Runner) error {
	if err := btrfs.CheckVolume(runner, plan.SnapshotSource, false); err != nil {
		return err
	}
	return nil
}

func validateConfigDestination(cfg *config.Config) (string, error) {
	if cfg.UsesDuplicacyStorage() {
		return validateDuplicacyStorage(cfg.Storage)
	}
	return "", configPathError(fmt.Sprintf("unsupported storage type %q", cfg.StorageType))
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
	dup := duplicacy.NewSetup(plan.WorkRoot, plan.RepositoryPath, plan.BackupTarget, false, runner)
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

func validateDuplicacyStorage(storage string) (string, error) {
	if storage == "" {
		return "", configPathError("storage must not be empty")
	}
	parsed, err := url.Parse(storage)
	if err != nil {
		return "", configPathError(fmt.Sprintf("duplicacy storage is not a valid URL-like storage target: %v", err))
	}
	if parsed.Scheme == "" {
		return validateLocalDuplicacyStorage(storage)
	}
	if parsed.Host == "" && parsed.Path == "" {
		return "", configPathError(fmt.Sprintf("duplicacy storage must include a target after the scheme (was %q)", storage))
	}
	return "Resolved", nil
}

func validateLocalDuplicacyStorage(storage string) (string, error) {
	if !filepath.IsAbs(storage) {
		return "", configPathError(fmt.Sprintf("duplicacy local storage must be an absolute path or a URL-like storage target (was %q)", storage))
	}
	info, err := os.Stat(storage)
	if err != nil {
		if os.IsNotExist(err) {
			parent := filepath.Dir(storage)
			if _, parentErr := os.Stat(parent); parentErr != nil {
				if os.IsNotExist(parentErr) {
					return "", configPathError(fmt.Sprintf("duplicacy local storage parent does not exist: %s", parent))
				}
				return "", configPathError(fmt.Sprintf("duplicacy local storage parent is not accessible: %v", parentErr))
			}
			return validateWritableDirectory(parent, "duplicacy local storage parent")
		}
		return "", configPathError(fmt.Sprintf("duplicacy local storage is not accessible: %v", err))
	}
	if !info.IsDir() {
		return "", configPathError(fmt.Sprintf("duplicacy local storage must be a directory: %s", storage))
	}
	return validateWritableDirectory(storage, "duplicacy local storage")
}

func validateWritableDirectory(path, description string) (string, error) {
	probe, err := os.CreateTemp(path, ".duplicacy-backup-config-validate-*")
	if err != nil {
		return "", configPathError(fmt.Sprintf("%s is not writable: %s", description, path))
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	return "Writable", nil
}

func validateDuplicacyStorageSecrets(storage string, sec *secrets.Secrets) error {
	required := duplicacyStorageRequiredKeys(storage)
	for _, key := range required {
		if sec == nil || strings.TrimSpace(sec.Keys[key]) == "" {
			return NewMessageError("storage %q requires %s in [targets.<name>.keys]", duplicacyStorageScheme(storage), strings.Join(required, " and "))
		}
	}
	return nil
}

func duplicacyStorageNeedsSecrets(storage string) bool {
	return len(duplicacyStorageRequiredKeys(storage)) > 0
}

func duplicacyStorageRequiredKeys(storage string) []string {
	parsed, err := url.Parse(storage)
	if err != nil {
		return nil
	}
	return requiredDuplicacyStorageKeys(parsed.Scheme)
}

func duplicacyStorageScheme(storage string) string {
	parsed, err := url.Parse(storage)
	if err != nil || parsed.Scheme == "" {
		return "local"
	}
	return parsed.Scheme
}

func duplicacyStorageSupportsFixPerms(cfg *config.Config) bool {
	return cfg != nil && cfg.UsesDuplicacyStorage() && isLocalDuplicacyStorage(cfg.Storage)
}

func isLocalDuplicacyStorage(storage string) bool {
	return strings.TrimSpace(storage) != "" && !strings.Contains(storage, "://")
}

func requiredDuplicacyStorageKeys(scheme string) []string {
	switch strings.ToLower(scheme) {
	case "s3":
		return []string{"s3_id", "s3_secret"}
	case "storj":
		return []string{"storj_key", "storj_passphrase"}
	default:
		return nil
	}
}

func configPathError(message string) error {
	return NewMessageError("%s", message)
}
