package workflow

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/btrfs"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
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
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Config File", Value: plan.ConfigFile},
	}

	cfg, err := planner.loadConfigWithOptions(plan, false)
	if err != nil {
		return "", err
	}
	resolved[1].Value = plan.ConfigFile
	plan.Target = cfg.Target
	plan.TargetType = cfg.TargetType
	plan.SnapshotSource = cfg.SourcePath
	resolved = append(resolved,
		SummaryLine{Label: "Source Path", Value: plan.SnapshotSource},
		SummaryLine{Label: "Destination", Value: cfg.Destination},
	)
	collector := newConfigValidationCollector([]SummaryLine{{Label: "Config", Value: "Valid"}})

	sourceAccessible, sourceStatus, err := validateConfigSourcePathAccess(plan.SnapshotSource)
	collector.addStatus("Source Path Access", sourceStatus, err)

	if sourceAccessible {
		collector.addStatus("Btrfs Source", "Valid", validateConfigSourceBtrfs(plan, planner.runner))
	} else {
		collector.addUnchecked("Btrfs Source")
	}

	collector.addStatus("Required Settings", "Valid", cfg.ValidateRequired(true, false))
	collector.addStatus("Health Thresholds", "Valid", cfg.ValidateThresholds())
	collector.addStatus("Threads", fmt.Sprintf("Valid (%d)", cfg.Threads), cfg.ValidateThreads())

	if cfg.Prune != "" {
		collector.addStatus("Prune Policy", "Valid", cfg.ValidatePrunePolicy())
	} else {
		collector.addStatic("Prune Policy", "Not configured")
	}

	targetSemanticsErr := cfg.ValidateTargetSemantics()
	if cfg.TargetType == targetLocal {
		localAccounts := "Not enabled"
		if cfg.AllowLocalAccounts && cfg.LocalOwner != "" && cfg.LocalGroup != "" {
			localAccounts = fmt.Sprintf("Validated (%s:%s)", cfg.LocalOwner, cfg.LocalGroup)
		}
		collector.addStatus("Local Accounts", localAccounts, targetSemanticsErr)
	} else {
		collector.addStatus("Target Settings", "Valid", targetSemanticsErr)
	}
	destinationStatus, destinationErr := validateConfigDestination(cfg)
	if destinationErr == nil {
		collector.addStatic("Destination Access", destinationStatus)
	} else {
		collector.addStatus("Destination Access", destinationStatus, destinationErr)
	}

	if plan.TargetType == targetRemote {
		_, err := planner.loadSecrets(plan)
		collector.addStatus("Secrets", "Valid", err)
	}

	if collector.failed() {
		output := formatConfigValidationOutput(fmt.Sprintf("Config validation failed for %s/%s", req.Source, plan.TargetName()), resolved, collector.lines, "Failed")
		return "", &ConfigCommandError{
			Message: fmt.Sprintf("Config validation failed for %s/%s", req.Source, plan.TargetName()),
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
	plan.TargetType = cfg.TargetType
	plan.ModeDisplay = modeDisplay(plan.TargetName(), plan.TargetType)
	plan.SnapshotSource = cfg.SourcePath
	plan.BackupTarget = JoinDestination(cfg.Destination, cfg.Repository)
	plan.Threads = cfg.Threads
	plan.PruneOptions = cfg.Prune
	plan.LocalOwner = cfg.LocalOwner
	plan.LocalGroup = cfg.LocalGroup

	lines := []SummaryLine{
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Config File", Value: plan.ConfigFile},
		{Label: "Source", Value: plan.SnapshotSource},
		{Label: "Destination", Value: plan.BackupTarget},
	}

	if cfg.Threads > 0 {
		lines = append(lines, SummaryLine{Label: "Threads", Value: fmt.Sprintf("%d", cfg.Threads)})
	}
	if cfg.Prune != "" {
		lines = append(lines, SummaryLine{Label: "Prune Policy", Value: cfg.Prune})
	}

	if plan.TargetType == targetRemote {
		sec, err := planner.loadSecrets(plan)
		if err != nil {
			return "", err
		}
		lines = append(lines,
			SummaryLine{Label: "Secrets File", Value: plan.SecretsFile},
			SummaryLine{Label: "Remote Access Key", Value: sec.MaskedID()},
			SummaryLine{Label: "Remote Secret Key", Value: sec.MaskedSecret()},
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
		plan.TargetType = cfg.TargetType
		plan.ModeDisplay = modeDisplay(plan.TargetName(), plan.TargetType)
		plan.SnapshotSource = cfg.SourcePath
	}
	lines := []SummaryLine{
		{Label: "Target", Value: plan.TargetName()},
		{Label: "Config Dir", Value: plan.ConfigDir},
		{Label: "Config File", Value: plan.ConfigFile},
		{Label: "Source Path", Value: plan.SnapshotSource},
		{Label: "Log Dir", Value: meta.LogDir},
	}
	if plan.TargetType == targetRemote {
		lines = append(lines[:3],
			append([]SummaryLine{
				{Label: "Secrets Dir", Value: plan.SecretsDir},
				{Label: "Secrets File", Value: plan.SecretsFile},
			}, lines[3:]...)...,
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
	var b strings.Builder
	b.WriteString(title)
	b.WriteByte('\n')
	for _, line := range lines {
		fmt.Fprintf(&b, "  %-20s : %s\n", line.Label, line.Value)
	}
	return b.String()
}

func formatConfigValidationOutput(title string, resolved, validation []SummaryLine, result string) string {
	enableColour := logger.IsTerminal(os.Stdout)
	var b strings.Builder
	b.WriteString(title)
	b.WriteByte('\n')
	writeConfigSection(&b, "Resolved", resolved, false, enableColour)
	writeConfigSection(&b, "Validation", validation, true, enableColour)
	fmt.Fprintf(&b, "  %-20s : %s\n", "Result", colourizeConfigValidationResult(result, enableColour))
	return b.String()
}

func writeConfigSection(b *strings.Builder, name string, lines []SummaryLine, semanticValues bool, enableColour bool) {
	fmt.Fprintf(b, "  Section: %s\n", name)
	for _, line := range lines {
		value := line.Value
		if semanticValues {
			value = colourizeConfigValidationValue(value, enableColour)
		}
		fmt.Fprintf(b, "    %-18s : %s\n", line.Label, value)
	}
}

func colourizeConfigValidationValue(value string, enableColour bool) string {
	switch {
	case strings.HasPrefix(value, "Invalid ("):
		return logger.ColourizeForLevel(logger.ERROR, value, enableColour)
	case value == "Not checked":
		return logger.ColourizeForLevel(logger.WARNING, value, enableColour)
	case value == "Valid",
		strings.HasPrefix(value, "Valid ("),
		value == "Readable",
		value == "Writable",
		strings.HasPrefix(value, "Resolved ("),
		strings.HasPrefix(value, "Parsed ("),
		strings.HasPrefix(value, "Validated ("):
		return logger.ColourizeForLevel(logger.SUCCESS, value, enableColour)
	default:
		return value
	}
}

func colourizeConfigValidationResult(value string, enableColour bool) string {
	switch value {
	case "Passed":
		return logger.ColourizeForLevel(logger.SUCCESS, value, enableColour)
	case "Warning":
		return logger.ColourizeForLevel(logger.WARNING, value, enableColour)
	case "Failed":
		return logger.ColourizeForLevel(logger.ERROR, value, enableColour)
	default:
		return value
	}
}

type configValidationCollector struct {
	lines  []SummaryLine
	errors []string
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

func (c *configValidationCollector) failed() bool {
	return len(c.errors) > 0
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
	switch cfg.TargetType {
	case targetLocal:
		return validateLocalDestination(cfg.Destination)
	case targetRemote:
		return validateRemoteDestination(cfg.Destination, cfg.RequiresNetwork)
	default:
		return "", configPathError(fmt.Sprintf("unsupported target type %q", cfg.TargetType))
	}
}

func validateLocalDestination(destination string) (string, error) {
	if destination == "" {
		return "", configPathError("destination must not be empty")
	}
	if !filepath.IsAbs(destination) {
		return "", configPathError(fmt.Sprintf("local destination must be an absolute path (was %q)", destination))
	}
	info, err := os.Stat(destination)
	if err != nil {
		if os.IsNotExist(err) {
			return "", configPathError(fmt.Sprintf("local destination does not exist: %s", destination))
		}
		return "", configPathError(fmt.Sprintf("local destination is not accessible: %v", err))
	}
	if !info.IsDir() {
		return "", configPathError(fmt.Sprintf("local destination must be a directory: %s", destination))
	}
	probe, err := os.CreateTemp(destination, ".duplicacy-backup-config-validate-*")
	if err != nil {
		return "", configPathError(fmt.Sprintf("local destination is not writable: %s", destination))
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	return "Writable", nil
}

func validateRemoteDestination(destination string, requiresNetwork bool) (string, error) {
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
		return "", configPathError(fmt.Sprintf("remote destination host could not be determined from %q", destination))
	}
	if !requiresNetwork {
		return fmt.Sprintf("Parsed (%s)", host), nil
	}
	addrs, err := resolveConfigDestinationHost(host)
	if err != nil {
		return "", configPathError(fmt.Sprintf("remote destination host could not be resolved: %s", host))
	}
	if len(addrs) == 0 {
		return "", configPathError(fmt.Sprintf("remote destination host resolved without any addresses: %s", host))
	}
	return fmt.Sprintf("Resolved (%s)", host), nil
}

func configPathError(message string) error {
	return NewMessageError("%s", message)
}
