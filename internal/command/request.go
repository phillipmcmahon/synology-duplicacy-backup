package command

import (
	"fmt"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

type ParseResult struct {
	Request *workflow.Request
	Output  string
	Handled bool
}

func ParseRequest(args []string, meta workflow.Metadata, rt workflow.Runtime) (*ParseResult, error) {
	if len(args) == 0 {
		return &ParseResult{Handled: true, Output: UsageText(meta, rt)}, nil
	}
	switch args[0] {
	case "backup", "prune", "cleanup-storage":
		return parseRuntimeCommandRequest(args[0], args[1:], meta, rt)
	}
	if len(args) > 0 && args[0] == "config" {
		return parseConfigRequest(args[1:], meta, rt)
	}
	if len(args) > 0 && args[0] == "diagnostics" {
		return parseDiagnosticsRequest(args[1:], meta, rt)
	}
	if len(args) > 0 && args[0] == "health" {
		return parseHealthRequest(args[1:], meta, rt)
	}
	if len(args) > 0 && args[0] == "notify" {
		return parseNotifyRequest(args[1:], meta, rt)
	}
	if len(args) > 0 && args[0] == "restore" {
		return parseRestoreRequest(args[1:], meta, rt)
	}
	if len(args) > 0 && args[0] == "rollback" {
		return parseRollbackRequest(args[1:], meta, rt)
	}
	if len(args) > 0 && args[0] == "update" {
		return parseUpdateRequest(args[1:], meta, rt)
	}

	if result := parseTopLevelMetaRequest(args, meta, rt); result != nil {
		return result, nil
	}

	if isOption(args[0]) {
		return nil, workflow.NewUsageRequestError("unknown top-level option %s; use a command such as backup, prune, or cleanup-storage", args[0])
	}
	return nil, workflow.NewUsageRequestError("unknown command %s", args[0])
}

func parseRuntimeCommandRequest(command string, args []string, meta workflow.Metadata, rt workflow.Runtime) (*ParseResult, error) {
	if result := parseHelpRequest(args, UsageText(meta, rt), FullUsageText(meta, rt)); result != nil {
		return result, nil
	}
	req, err := parseRuntimeCommandFlags(command, args)
	if err != nil {
		return nil, err
	}
	switch command {
	case "backup":
		req.DoBackup = true
	case "prune":
		req.DoPrune = true
	case "cleanup-storage":
		req.DoCleanupStore = true
	default:
		return nil, workflow.NewUsageRequestError("unknown command %s", command)
	}
	req.DeriveModes()
	if err := validateTargetAndLabel(req); err != nil {
		return nil, err
	}

	return &ParseResult{Request: req}, nil
}

func parseDiagnosticsRequest(args []string, meta workflow.Metadata, rt workflow.Runtime) (*ParseResult, error) {
	if result := parseHelpRequest(args, DiagnosticsUsageText(meta, rt), FullDiagnosticsUsageText(meta, rt)); result != nil {
		return result, nil
	}
	req, err := parseDiagnosticsFlags(args)
	if err != nil {
		return nil, err
	}
	req.DiagnosticsCommand = "diagnostics"
	if err := validateTargetAndLabel(req); err != nil {
		return nil, err
	}

	return &ParseResult{Request: req}, nil
}

func parseHealthRequest(args []string, meta workflow.Metadata, rt workflow.Runtime) (*ParseResult, error) {
	if len(args) == 0 {
		return nil, workflow.NewUsageRequestError("health command required")
	}
	if result := parseHelpRequest(args, UsageText(meta, rt), FullUsageText(meta, rt)); result != nil {
		return result, nil
	}

	action := args[0]
	switch action {
	case "status", "doctor", "verify":
	default:
		return nil, workflow.NewUsageRequestError("unknown health command %s", action)
	}

	req, err := parseHealthFlags(args[1:])
	if err != nil {
		return nil, err
	}
	req.HealthCommand = action
	if err := validateTargetAndLabel(req); err != nil {
		return nil, err
	}

	return &ParseResult{Request: req}, nil
}

func parseConfigRequest(args []string, meta workflow.Metadata, rt workflow.Runtime) (*ParseResult, error) {
	if len(args) == 0 {
		return &ParseResult{Handled: true, Output: ConfigUsageText(meta, rt)}, nil
	}
	if result := parseHelpRequest(args, ConfigUsageText(meta, rt), FullConfigUsageText(meta, rt)); result != nil {
		return result, nil
	}

	action := args[0]
	switch action {
	case "validate", "explain", "paths":
	default:
		return nil, workflow.NewUsageRequestError("unknown config command %s", action)
	}

	req, err := parseConfigFlags(args[1:])
	if err != nil {
		return nil, err
	}
	req.ConfigCommand = action
	if err := validateTargetAndLabel(req); err != nil {
		return nil, err
	}

	return &ParseResult{Request: req}, nil
}

func parseNotifyRequest(args []string, meta workflow.Metadata, rt workflow.Runtime) (*ParseResult, error) {
	if len(args) == 0 {
		return &ParseResult{Handled: true, Output: NotifyUsageText(meta, rt)}, nil
	}
	if result := parseHelpRequest(args, NotifyUsageText(meta, rt), FullNotifyUsageText(meta, rt)); result != nil {
		return result, nil
	}

	action := args[0]
	switch action {
	case "test":
	default:
		return nil, workflow.NewUsageRequestError("unknown notify command %s", action)
	}

	req, err := parseNotifyFlags(args[1:])
	if err != nil {
		return nil, err
	}
	req.NotifyCommand = action
	if action == "test" && req.Source == "update" {
		req.Source = ""
		req.NotifyScope = "update"
		if req.RequestedTarget != "" {
			return nil, workflow.NewUsageRequestError("notify test update does not use --target")
		}
		return &ParseResult{Request: req}, nil
	}
	if req.NotifyEvent != "" {
		return nil, workflow.NewUsageRequestError("--event is only supported for notify test update")
	}
	if err := validateTargetAndLabel(req); err != nil {
		return nil, err
	}

	return &ParseResult{Request: req}, nil
}

func parseRestoreRequest(args []string, meta workflow.Metadata, rt workflow.Runtime) (*ParseResult, error) {
	if len(args) == 0 {
		return &ParseResult{Handled: true, Output: RestoreUsageText(meta, rt)}, nil
	}
	if result := parseHelpRequest(args, RestoreUsageText(meta, rt), FullRestoreUsageText(meta, rt)); result != nil {
		return result, nil
	}

	action := args[0]
	switch action {
	case "plan", "list-revisions", "run", "select":
	default:
		return nil, workflow.NewUsageRequestError("unknown restore command %s", action)
	}

	req, err := parseRestoreFlags(action, args[1:])
	if err != nil {
		return nil, err
	}
	req.RestoreCommand = action
	switch action {
	case "run":
		if req.RestoreRevision <= 0 {
			return nil, workflow.NewUsageRequestError("restore %s requires --revision <id>", action)
		}
	case "list-revisions":
		if req.RestoreLimit == 0 {
			req.RestoreLimit = 50
		}
	}
	if err := validateTargetAndLabel(req); err != nil {
		return nil, err
	}

	return &ParseResult{Request: req}, nil
}

func parseRollbackRequest(args []string, meta workflow.Metadata, rt workflow.Runtime) (*ParseResult, error) {
	if result := parseHelpRequest(args, RollbackUsageText(meta, rt), FullRollbackUsageText(meta, rt)); result != nil {
		return result, nil
	}
	req, err := parseRollbackFlags(args)
	if err != nil {
		return nil, err
	}
	req.RollbackCommand = "rollback"
	return &ParseResult{Request: req}, nil
}

func parseUpdateRequest(args []string, meta workflow.Metadata, rt workflow.Runtime) (*ParseResult, error) {
	if result := parseHelpRequest(args, UpdateUsageText(meta, rt), FullUpdateUsageText(meta, rt)); result != nil {
		return result, nil
	}

	req, err := parseUpdateFlags(args)
	if err != nil {
		return nil, err
	}
	req.UpdateCommand = "update"
	return &ParseResult{Request: req}, nil
}

func parseRuntimeCommandFlags(command string, args []string) (*workflow.Request, error) {
	req := &workflow.Request{}
	err := parseSourceFlags(args, req, sharedFlagOptions{
		target:      true,
		dryRun:      true,
		verbose:     true,
		jsonSummary: true,
		configDir:   true,
		secretsDir:  true,
	}, func(args []string, index *int, req *workflow.Request) (bool, error) {
		switch args[*index] {
		case "--force":
			if command != "prune" {
				return false, nil
			}
			req.ForcePrune = true
			return true, nil
		}
		return false, nil
	})
	return req, err
}

func parseNotifyFlags(args []string) (*workflow.Request, error) {
	req := &workflow.Request{
		NotifyProvider: "all",
		NotifySeverity: "warning",
	}
	err := parseSourceFlags(args, req, sharedFlagOptions{
		target:      true,
		dryRun:      true,
		jsonSummary: true,
		configDir:   true,
		secretsDir:  true,
	}, func(args []string, index *int, req *workflow.Request) (bool, error) {
		switch args[*index] {
		case "--provider":
			value, err := consumeRequiredValue(args, index, "--provider")
			if err != nil {
				return false, err
			}
			req.NotifyProvider = value
			return true, nil
		case "--severity":
			value, err := consumeRequiredValue(args, index, "--severity")
			if err != nil {
				return false, err
			}
			req.NotifySeverity = value
			return true, nil
		case "--summary":
			value, err := consumeRequiredValue(args, index, "--summary")
			if err != nil {
				return false, err
			}
			req.NotifySummary = value
			return true, nil
		case "--message":
			value, err := consumeRequiredValue(args, index, "--message")
			if err != nil {
				return false, err
			}
			req.NotifyMessage = value
			return true, nil
		case "--event":
			value, err := consumeRequiredValue(args, index, "--event")
			if err != nil {
				return false, err
			}
			req.NotifyEvent = value
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	if err := validateNotifyProvider(req.NotifyProvider); err != nil {
		return nil, err
	}
	if err := validateNotifySeverity(req.NotifySeverity); err != nil {
		return nil, err
	}
	if err := validateNotifyEvent(req.NotifyEvent); err != nil {
		return nil, err
	}
	return req, nil
}

func parseConfigFlags(args []string) (*workflow.Request, error) {
	req := &workflow.Request{}
	return req, parseSourceFlags(args, req, sharedFlagOptions{
		target:     true,
		verbose:    true,
		configDir:  true,
		secretsDir: true,
	}, nil)
}

func parseHealthFlags(args []string) (*workflow.Request, error) {
	req := &workflow.Request{}
	return req, parseSourceFlags(args, req, sharedFlagOptions{
		target:      true,
		verbose:     true,
		jsonSummary: true,
		configDir:   true,
		secretsDir:  true,
	}, nil)
}

func parseDiagnosticsFlags(args []string) (*workflow.Request, error) {
	req := &workflow.Request{}
	return req, parseSourceFlags(args, req, sharedFlagOptions{
		target:      true,
		jsonSummary: true,
		configDir:   true,
		secretsDir:  true,
	}, nil)
}

func parseRestoreFlags(action string, args []string) (*workflow.Request, error) {
	req := &workflow.Request{}
	opts := sharedFlagOptions{
		target:     true,
		configDir:  true,
		secretsDir: true,
	}
	switch action {
	case "list-revisions":
		opts.jsonSummary = true
	case "run":
		opts.dryRun = true
	}
	return req, parseSourceFlagsWithLabel(args, req, sharedFlagOptions{
		target:      opts.target,
		dryRun:      opts.dryRun,
		jsonSummary: opts.jsonSummary,
		configDir:   opts.configDir,
		secretsDir:  opts.secretsDir,
	}, func(args []string, index *int, req *workflow.Request) (bool, error) {
		switch args[*index] {
		case "--workspace":
			value, err := consumeRequiredValue(args, index, "--workspace")
			if err != nil {
				return false, err
			}
			req.RestoreWorkspace = value
			return true, nil
		case "--workspace-root":
			if action != "run" && action != "select" {
				return false, nil
			}
			value, err := consumeRequiredValue(args, index, "--workspace-root")
			if err != nil {
				return false, err
			}
			req.RestoreWorkspaceRoot = value
			return true, nil
		case "--workspace-template":
			if action != "run" && action != "select" {
				return false, nil
			}
			value, err := consumeRequiredValue(args, index, "--workspace-template")
			if err != nil {
				return false, err
			}
			req.RestoreWorkspaceTemplate = value
			return true, nil
		case "--revision":
			if action != "run" {
				return false, nil
			}
			value, err := consumeRequiredValue(args, index, "--revision")
			if err != nil {
				return false, err
			}
			revision, err := parsePositiveInt(value, "--revision")
			if err != nil {
				return false, err
			}
			req.RestoreRevision = revision
			return true, nil
		case "--path":
			if action != "run" {
				return false, nil
			}
			value, err := consumeRequiredValue(args, index, "--path")
			if err != nil {
				return false, err
			}
			req.RestorePath = value
			return true, nil
		case "--path-prefix":
			if action != "select" {
				return false, nil
			}
			value, err := consumeRequiredValue(args, index, "--path-prefix")
			if err != nil {
				return false, err
			}
			req.RestorePathPrefix = value
			return true, nil
		case "--limit":
			if action != "list-revisions" {
				return false, nil
			}
			value, err := consumeRequiredValue(args, index, "--limit")
			if err != nil {
				return false, err
			}
			limit, err := parsePositiveInt(value, "--limit")
			if err != nil {
				return false, err
			}
			req.RestoreLimit = limit
			return true, nil
		case "--yes":
			if action != "run" {
				return false, nil
			}
			req.RestoreYes = true
			return true, nil
		}
		return false, nil
	})
}

func parseSourceFlagsWithLabel(args []string, req *workflow.Request, opts sharedFlagOptions, extra extraFlagParser) error {
	if err := parseSourceFlags(args, req, opts, extra); err != nil {
		if strings.Contains(err.Error(), "source directory required") {
			return workflow.NewUsageRequestError("backup label required")
		}
		return err
	}
	return nil
}

func parseRollbackFlags(args []string) (*workflow.Request, error) {
	req := &workflow.Request{}
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		return nil, workflow.NewUsageRequestError("unexpected extra arguments: %s", strings.Join(args, " "))
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--yes":
			req.RollbackYes = true
		case "--check-only":
			req.RollbackCheckOnly = true
		case "--version":
			value, err := consumeRequiredValue(args, &i, "--version")
			if err != nil {
				return nil, err
			}
			req.RollbackVersion = value
		default:
			if isOption(args[i]) {
				return nil, workflow.NewUsageRequestError("unknown option %s", args[i])
			}
			return nil, workflow.NewUsageRequestError("unexpected extra arguments: %s", strings.Join(args[i:], " "))
		}
	}

	return req, nil
}

type extraFlagParser func(args []string, index *int, req *workflow.Request) (bool, error)

func parseSourceFlags(args []string, req *workflow.Request, opts sharedFlagOptions, extra extraFlagParser) error {
	var positional []string

	for i := 0; i < len(args); i++ {
		if extra != nil {
			handled, err := extra(args, &i, req)
			if err != nil {
				return err
			}
			if handled {
				continue
			}
		}
		handled, err := consumeSharedFlag(args, &i, req, sharedFlagOptions{
			target:      opts.target,
			dryRun:      opts.dryRun,
			verbose:     opts.verbose,
			jsonSummary: opts.jsonSummary,
			configDir:   opts.configDir,
			secretsDir:  opts.secretsDir,
		})
		if err != nil {
			return err
		}
		if handled {
			continue
		}
		if isOption(args[i]) {
			return workflow.NewUsageRequestError("unknown option %s", args[i])
		}
		positional = append(positional, args[i])
	}

	source, err := parseSourcePositional(positional)
	if err != nil {
		return err
	}
	req.Source = source
	return nil
}

func parseUpdateFlags(args []string) (*workflow.Request, error) {
	req := &workflow.Request{UpdateKeep: 2}
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		return nil, workflow.NewUsageRequestError("unexpected extra arguments: %s", strings.Join(args, " "))
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--yes":
			req.UpdateYes = true
		case "--check-only":
			req.UpdateCheckOnly = true
		case "--force":
			req.UpdateForce = true
		case "--keep":
			value, err := consumeRequiredValue(args, &i, "--keep")
			if err != nil {
				return nil, err
			}
			keep, err := parseNonNegativeInt(value, "--keep")
			if err != nil {
				return nil, err
			}
			req.UpdateKeep = keep
		case "--version":
			value, err := consumeRequiredValue(args, &i, "--version")
			if err != nil {
				return nil, err
			}
			req.UpdateVersion = value
		case "--attestations":
			value, err := consumeRequiredValue(args, &i, "--attestations")
			if err != nil {
				return nil, err
			}
			switch value {
			case "off", "auto", "required":
				req.UpdateAttestations = value
			default:
				return nil, workflow.NewUsageRequestError("--attestations must be one of: off, auto, required")
			}
		case "--config-dir":
			value, err := consumeRequiredValue(args, &i, "--config-dir")
			if err != nil {
				return nil, err
			}
			req.ConfigDir = value
		default:
			if isOption(args[i]) {
				return nil, workflow.NewUsageRequestError("unknown option %s", args[i])
			}
			return nil, workflow.NewUsageRequestError("unexpected extra arguments: %s", strings.Join(args[i:], " "))
		}
	}

	return req, nil
}

func parseTopLevelMetaRequest(args []string, meta workflow.Metadata, rt workflow.Runtime) *ParseResult {
	for _, arg := range args {
		switch arg {
		case "--help":
			return &ParseResult{Handled: true, Output: UsageText(meta, rt)}
		case "--help-full":
			return &ParseResult{Handled: true, Output: FullUsageText(meta, rt)}
		case "--version", "-v":
			return &ParseResult{Handled: true, Output: VersionText(meta)}
		}
	}
	return nil
}

func parseHelpRequest(args []string, usageText string, fullUsageText string) *ParseResult {
	for _, arg := range args {
		switch arg {
		case "--help":
			return &ParseResult{Handled: true, Output: usageText}
		case "--help-full":
			return &ParseResult{Handled: true, Output: fullUsageText}
		}
	}
	return nil
}

type sharedFlagOptions struct {
	target      bool
	dryRun      bool
	verbose     bool
	jsonSummary bool
	configDir   bool
	secretsDir  bool
}

func consumeSharedFlag(args []string, index *int, req *workflow.Request, opts sharedFlagOptions) (bool, error) {
	switch args[*index] {
	case "--target":
		if !opts.target {
			return false, nil
		}
		value, err := consumeRequiredValue(args, index, "--target")
		if err != nil {
			return false, err
		}
		req.RequestedTarget = value
		return true, nil
	case "--dry-run":
		if !opts.dryRun {
			return false, nil
		}
		req.DryRun = true
		return true, nil
	case "--verbose":
		if !opts.verbose {
			return false, nil
		}
		req.Verbose = true
		return true, nil
	case "--json-summary":
		if !opts.jsonSummary {
			return false, nil
		}
		req.JSONSummary = true
		return true, nil
	case "--config-dir":
		if !opts.configDir {
			return false, nil
		}
		value, err := consumeRequiredValue(args, index, "--config-dir")
		if err != nil {
			return false, err
		}
		req.ConfigDir = value
		return true, nil
	case "--secrets-dir":
		if !opts.secretsDir {
			return false, nil
		}
		value, err := consumeRequiredValue(args, index, "--secrets-dir")
		if err != nil {
			return false, err
		}
		req.SecretsDir = value
		return true, nil
	default:
		return false, nil
	}
}

func consumeRequiredValue(args []string, index *int, flag string) (string, error) {
	if *index+1 >= len(args) {
		return "", workflow.NewUsageRequestError("%s requires a value", flag)
	}
	*index++
	return args[*index], nil
}

func parseSourcePositional(positional []string) (string, error) {
	if len(positional) < 1 {
		return "", workflow.NewUsageRequestError("source directory required")
	}
	if len(positional) > 1 {
		return "", workflow.NewUsageRequestError("unexpected extra arguments: %s", strings.Join(positional[1:], " "))
	}
	return positional[0], nil
}

func parseNonNegativeInt(value string, flag string) (int, error) {
	var parsed int
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, workflow.NewUsageRequestError("%s must be a non-negative integer", flag)
		}
		parsed = parsed*10 + int(ch-'0')
	}
	return parsed, nil
}

func parsePositiveInt(value string, flag string) (int, error) {
	parsed, err := parseNonNegativeInt(value, flag)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, workflow.NewUsageRequestError("%s must be a positive integer", flag)
	}
	return parsed, nil
}

func validateTargetAndLabel(req *workflow.Request) error {
	if req.RequestedTarget == "" {
		return workflow.NewRequestError("--target is required")
	}
	if err := workflow.ValidateTargetName(req.RequestedTarget); err != nil {
		return err
	}
	return validateLabel(req.Source)
}

func validateLabel(source string) error {
	if err := workflow.ValidateLabel(source); err != nil {
		return fmt.Errorf("invalid source label: %w", err)
	}
	return nil
}

func validateNotifyProvider(provider string) error {
	switch strings.TrimSpace(provider) {
	case "", "all", "webhook", "ntfy":
		return nil
	default:
		return workflow.NewRequestError("unsupported notify provider %q; expected all, webhook, or ntfy", provider)
	}
}

func validateNotifySeverity(severity string) error {
	switch strings.TrimSpace(severity) {
	case "", "warning", "critical", "info":
		return nil
	default:
		return workflow.NewRequestError("unsupported notify severity %q; expected warning, critical, or info", severity)
	}
}

func validateNotifyEvent(event string) error {
	if strings.TrimSpace(event) == "" {
		return nil
	}
	if notify.IsKnownEvent(event) {
		return nil
	}
	return workflow.NewUsageRequestError("unsupported notify event %q", event)
}

func isOption(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}
