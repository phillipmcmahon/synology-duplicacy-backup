package command

import (
	"fmt"
	"strings"

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
	if len(args) > 0 && args[0] == "config" {
		return parseConfigRequest(args[1:], meta, rt)
	}
	if len(args) > 0 && args[0] == "health" {
		return parseHealthRequest(args[1:], meta, rt)
	}
	if len(args) > 0 && args[0] == "notify" {
		return parseNotifyRequest(args[1:], meta, rt)
	}
	if len(args) > 0 && args[0] == "update" {
		return parseUpdateRequest(args[1:], meta, rt)
	}

	if result := parseTopLevelMetaRequest(args, meta, rt); result != nil {
		return result, nil
	}

	req, err := parseFlags(args)
	if err != nil {
		return nil, err
	}

	req.DeriveModes()

	if err := req.ValidateCombos(); err != nil {
		return nil, err
	}
	if err := validateLabel(req.Source); err != nil {
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

func parseFlags(args []string) (*workflow.Request, error) {
	req := &workflow.Request{}
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--backup":
			req.DoBackup = true
		case "--prune":
			req.DoPrune = true
		case "--cleanup-storage":
			req.DoCleanupStore = true
		case "--fix-perms":
			req.FixPerms = true
		case "--force-prune":
			req.ForcePrune = true
		default:
			handled, err := consumeSharedFlag(args, &i, req, sharedFlagOptions{
				target:      true,
				dryRun:      true,
				verbose:     true,
				jsonSummary: true,
				configDir:   true,
				secretsDir:  true,
			})
			if err != nil {
				return nil, err
			}
			if handled {
				continue
			}
			if isOption(args[i]) {
				return nil, workflow.NewUsageRequestError("unknown option %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	source, err := parseSourcePositional(positional)
	if err != nil {
		return nil, err
	}
	req.Source = source
	return req, nil
}

func parseNotifyFlags(args []string) (*workflow.Request, error) {
	req := &workflow.Request{
		NotifyProvider: "all",
		NotifySeverity: "warning",
	}
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--provider":
			if i+1 >= len(args) {
				return nil, workflow.NewUsageRequestError("--provider requires a value")
			}
			i++
			req.NotifyProvider = args[i]
		case "--severity":
			if i+1 >= len(args) {
				return nil, workflow.NewUsageRequestError("--severity requires a value")
			}
			i++
			req.NotifySeverity = args[i]
		case "--summary":
			if i+1 >= len(args) {
				return nil, workflow.NewUsageRequestError("--summary requires a value")
			}
			i++
			req.NotifySummary = args[i]
		case "--message":
			if i+1 >= len(args) {
				return nil, workflow.NewUsageRequestError("--message requires a value")
			}
			i++
			req.NotifyMessage = args[i]
		case "--event":
			if i+1 >= len(args) {
				return nil, workflow.NewUsageRequestError("--event requires a value")
			}
			i++
			req.NotifyEvent = args[i]
		default:
			handled, err := consumeSharedFlag(args, &i, req, sharedFlagOptions{
				target:      true,
				dryRun:      true,
				jsonSummary: true,
				configDir:   true,
				secretsDir:  true,
			})
			if err != nil {
				return nil, err
			}
			if handled {
				continue
			}
			if isOption(args[i]) {
				return nil, workflow.NewUsageRequestError("unknown option %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	source, err := parseSourcePositional(positional)
	if err != nil {
		return nil, err
	}
	req.Source = source
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
	var positional []string

	for i := 0; i < len(args); i++ {
		handled, err := consumeSharedFlag(args, &i, req, sharedFlagOptions{
			target:     true,
			verbose:    true,
			configDir:  true,
			secretsDir: true,
		})
		if err != nil {
			return nil, err
		}
		if handled {
			continue
		}
		if isOption(args[i]) {
			return nil, workflow.NewUsageRequestError("unknown option %s", args[i])
		}
		positional = append(positional, args[i])
	}

	source, err := parseSourcePositional(positional)
	if err != nil {
		return nil, err
	}
	req.Source = source
	return req, nil
}

func parseHealthFlags(args []string) (*workflow.Request, error) {
	req := &workflow.Request{}
	var positional []string

	for i := 0; i < len(args); i++ {
		handled, err := consumeSharedFlag(args, &i, req, sharedFlagOptions{
			target:      true,
			verbose:     true,
			jsonSummary: true,
			configDir:   true,
			secretsDir:  true,
		})
		if err != nil {
			return nil, err
		}
		if handled {
			continue
		}
		if isOption(args[i]) {
			return nil, workflow.NewUsageRequestError("unknown option %s", args[i])
		}
		positional = append(positional, args[i])
	}

	source, err := parseSourcePositional(positional)
	if err != nil {
		return nil, err
	}
	req.Source = source
	return req, nil
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
		return fmt.Errorf("Invalid source label: %w", err)
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
	switch strings.TrimSpace(event) {
	case "", "update_check_failed", "update_download_failed", "update_checksum_failed", "update_attestation_failed", "update_install_failed", "update_install_succeeded", "update_already_current", "update_reinstall_requested":
		return nil
	default:
		return workflow.NewUsageRequestError("unsupported notify event %q", event)
	}
}

func isOption(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}
