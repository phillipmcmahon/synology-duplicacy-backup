package workflow

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
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

var resolveConfigDestinationHost = func(host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(context.Background(), host)
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
	plan.BackupTarget = JoinDestination(cfg.StorageType, cfg.Destination, cfg.Repository)
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
	secretsChecked := !cfg.UsesObjectStorage()
	targetSemanticsErr := cfg.ValidateTargetSemantics()
	if cfg.UsesObjectStorage() && privilegedValidation {
		secretsChecked = true
		sec, secretsErr = planner.loadSecrets(plan)
	}

	if sourceAccessible && destinationErr == nil {
		switch {
		case cfg.UsesFilesystem():
			repoStatus, repoHint, repoErr = validateConfigRepository(plan, cfg, planner.runner, sec)
		case cfg.UsesObjectStorage() && secretsChecked && secretsErr == nil:
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
		collector.addStatic("Destination Access", destinationStatus)
	} else {
		collector.addStatus("Destination Access", destinationStatus, destinationErr)
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
	if cfg.UsesObjectStorage() {
		if secretsChecked {
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
	plan.BackupTarget = JoinDestination(cfg.StorageType, cfg.Destination, cfg.Repository)
	plan.Threads = cfg.Threads
	plan.Filter = cfg.Filter
	plan.PruneOptions = cfg.Prune
	plan.LocalOwner = cfg.LocalOwner
	plan.LocalGroup = cfg.LocalGroup

	lines := []SummaryLine{
		{Label: "Label", Value: req.Source},
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Type", Value: cfg.StorageType},
		{Label: "Location", Value: cfg.Location},
		{Label: "Config File", Value: plan.ConfigFile},
		{Label: "Source", Value: plan.SnapshotSource},
		{Label: "Destination", Value: plan.BackupTarget},
	}

	if cfg.Threads > 0 {
		lines = append(lines, SummaryLine{Label: "Threads", Value: fmt.Sprintf("%d", cfg.Threads)})
	}
	if cfg.Filter != "" {
		lines = append(lines, SummaryLine{Label: "Filter", Value: cfg.Filter})
	}
	if cfg.Prune != "" {
		lines = append(lines, SummaryLine{Label: "Prune Policy", Value: cfg.Prune})
	}

	if cfg.UsesObjectStorage() {
		lines = append(lines,
			SummaryLine{Label: "Secrets File", Value: plan.SecretsFile},
		)
	} else {
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
	}
	lines := []SummaryLine{
		{Label: "Label", Value: req.Source},
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Type", Value: plan.StorageType},
		{Label: "Location", Value: plan.Location},
		{Label: "Config Dir", Value: plan.ConfigDir},
		{Label: "Config File", Value: plan.ConfigFile},
		{Label: "Source Path", Value: plan.SnapshotSource},
		{Label: "Log Dir", Value: meta.LogDir},
	}
	if plan.UsesObjectStorage() {
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
	switch {
	case cfg.UsesFilesystem():
		return validateFilesystemDestination(cfg.Destination)
	case cfg.UsesObjectStorage():
		return validateObjectDestination(cfg.Destination)
	default:
		return "", configPathError(fmt.Sprintf("unsupported storage type %q", cfg.StorageType))
	}
}

func validateConfigRepository(plan *Plan, cfg *config.Config, runner execpkg.Runner, sec *secrets.Secrets) (string, string, error) {
	if cfg.UsesFilesystem() {
		if status, hint, err := validateLocalRepositoryReadiness(plan.BackupTarget); err != nil || status == "Not initialized" {
			return status, hint, err
		}
	}

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

func validateLocalRepositoryReadiness(repoPath string) (string, string, error) {
	if repoPath == "" {
		return "", "", configPathError("repository path must not be empty")
	}
	info, err := os.Stat(repoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "Not initialized", "initialize the repository before running backups", nil
		}
		return "", "", configPathError(fmt.Sprintf("local repository path is not accessible: %v", err))
	}
	if !info.IsDir() {
		return "", "", configPathError(fmt.Sprintf("local repository path must be a directory: %s", repoPath))
	}
	snapshotsDir := filepath.Join(repoPath, "snapshots")
	if snapshotInfo, statErr := os.Stat(snapshotsDir); statErr != nil || !snapshotInfo.IsDir() {
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", "", configPathError(fmt.Sprintf("local repository snapshots directory is not accessible: %v", statErr))
		}
		return "Not initialized", "initialize the repository before running backups", nil
	}
	return "Valid", "", nil
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

func validateFilesystemDestination(destination string) (string, error) {
	if destination == "" {
		return "", configPathError("destination must not be empty")
	}
	if !filepath.IsAbs(destination) {
		return "", configPathError(fmt.Sprintf("filesystem destination must be an absolute path (was %q)", destination))
	}
	info, err := os.Stat(destination)
	if err != nil {
		if os.IsNotExist(err) {
			return "", configPathError(fmt.Sprintf("filesystem destination does not exist: %s", destination))
		}
		return "", configPathError(fmt.Sprintf("filesystem destination is not accessible: %v", err))
	}
	if !info.IsDir() {
		return "", configPathError(fmt.Sprintf("filesystem destination must be a directory: %s", destination))
	}
	probe, err := os.CreateTemp(destination, ".duplicacy-backup-config-validate-*")
	if err != nil {
		return "", configPathError(fmt.Sprintf("filesystem destination is not writable: %s", destination))
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	return "Writable", nil
}

func validateObjectDestination(destination string) (string, error) {
	if destination == "" {
		return "", configPathError("destination must not be empty")
	}
	parsed, err := url.Parse(destination)
	if err != nil {
		return "", configPathError(fmt.Sprintf("remote destination is not a valid URL-like storage target: %v", err))
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", configPathError(fmt.Sprintf("remote destination must include a scheme and host (was %q)", destination))
	}
	host := parsed.Hostname()
	if host == "" {
		return "", configPathError(fmt.Sprintf("object destination host could not be determined from %q", destination))
	}
	addrs, err := resolveConfigDestinationHost(host)
	if err != nil {
		return "", configPathError(fmt.Sprintf("object destination host could not be resolved: %s", host))
	}
	if len(addrs) == 0 {
		return "", configPathError(fmt.Sprintf("object destination host resolved without any addresses: %s", host))
	}
	return "Resolved", nil
}

func configPathError(message string) error {
	return NewMessageError("%s", message)
}
