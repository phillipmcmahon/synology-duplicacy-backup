package workflow

import (
	"fmt"
	"strings"
)

// RequestError describes a request-parsing or request-validation failure.
// ShowUsage marks errors that should be followed by usage text.
type RequestError struct {
	message   string
	ShowUsage bool
}

func (e *RequestError) Error() string {
	return e.message
}

func NewRequestError(format string, args ...interface{}) *RequestError {
	return &RequestError{message: fmt.Sprintf(format, args...)}
}

func NewUsageRequestError(format string, args ...interface{}) *RequestError {
	return &RequestError{message: fmt.Sprintf(format, args...), ShowUsage: true}
}

type Request struct {
	ConfigCommand   string
	HealthCommand   string
	NotifyCommand   string
	FixPerms        bool
	ForcePrune      bool
	RequestedTarget string
	DryRun          bool
	Verbose         bool
	JSONSummary     bool
	ConfigDir       string
	SecretsDir      string
	Source          string
	NotifyProvider  string
	NotifySeverity  string
	NotifySummary   string
	NotifyMessage   string
	DoBackup        bool
	DoPrune         bool
	DoCleanupStore  bool
	FixPermsOnly    bool
	DefaultNotice   string
}

func (r *Request) Target() string {
	if r != nil {
		return r.RequestedTarget
	}
	return ""
}

type ParseResult struct {
	Request *Request
	Output  string
	Handled bool
}

func ParseRequest(args []string, meta Metadata, rt Runtime) (*ParseResult, error) {
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

	if result := parseTopLevelMetaRequest(args, meta, rt); result != nil {
		return result, nil
	}

	req, err := parseFlags(args)
	if err != nil {
		return nil, err
	}

	req.deriveModes()

	if err := req.validateCombos(); err != nil {
		return nil, err
	}
	if err := validateLabel(req.Source); err != nil {
		return nil, err
	}

	return &ParseResult{Request: req}, nil
}

func parseHealthRequest(args []string, meta Metadata, rt Runtime) (*ParseResult, error) {
	if len(args) == 0 {
		return nil, NewUsageRequestError("health command required")
	}
	if result := parseHelpRequest(args, UsageText(meta, rt), FullUsageText(meta, rt)); result != nil {
		return result, nil
	}

	action := args[0]
	switch action {
	case "status", "doctor", "verify":
	default:
		return nil, NewUsageRequestError("unknown health command %s", action)
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

func parseConfigRequest(args []string, meta Metadata, rt Runtime) (*ParseResult, error) {
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
		return nil, NewUsageRequestError("unknown config command %s", action)
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

func parseNotifyRequest(args []string, meta Metadata, rt Runtime) (*ParseResult, error) {
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
		return nil, NewUsageRequestError("unknown notify command %s", action)
	}

	req, err := parseNotifyFlags(args[1:])
	if err != nil {
		return nil, err
	}
	req.NotifyCommand = action
	if err := validateTargetAndLabel(req); err != nil {
		return nil, err
	}

	return &ParseResult{Request: req}, nil
}

func parseFlags(args []string) (*Request, error) {
	req := &Request{}
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
				return nil, NewUsageRequestError("unknown option %s", args[i])
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

func parseNotifyFlags(args []string) (*Request, error) {
	req := &Request{
		NotifyProvider: "all",
		NotifySeverity: "warning",
	}
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--provider":
			if i+1 >= len(args) {
				return nil, NewUsageRequestError("--provider requires a value")
			}
			i++
			req.NotifyProvider = args[i]
		case "--severity":
			if i+1 >= len(args) {
				return nil, NewUsageRequestError("--severity requires a value")
			}
			i++
			req.NotifySeverity = args[i]
		case "--summary":
			if i+1 >= len(args) {
				return nil, NewUsageRequestError("--summary requires a value")
			}
			i++
			req.NotifySummary = args[i]
		case "--message":
			if i+1 >= len(args) {
				return nil, NewUsageRequestError("--message requires a value")
			}
			i++
			req.NotifyMessage = args[i]
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
				return nil, NewUsageRequestError("unknown option %s", args[i])
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
	return req, nil
}

func parseConfigFlags(args []string) (*Request, error) {
	req := &Request{}
	var positional []string

	for i := 0; i < len(args); i++ {
		handled, err := consumeSharedFlag(args, &i, req, sharedFlagOptions{
			target:     true,
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
			return nil, NewUsageRequestError("unknown option %s", args[i])
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

func validateNotifyProvider(provider string) error {
	switch strings.TrimSpace(provider) {
	case "", "all", "webhook", "ntfy":
		return nil
	default:
		return NewRequestError("unsupported notify provider %q; expected all, webhook, or ntfy", provider)
	}
}

func validateNotifySeverity(severity string) error {
	switch strings.TrimSpace(severity) {
	case "", "warning", "critical", "info":
		return nil
	default:
		return NewRequestError("unsupported notify severity %q; expected warning, critical, or info", severity)
	}
}

func parseHealthFlags(args []string) (*Request, error) {
	req := &Request{}
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
			return nil, NewUsageRequestError("unknown option %s", args[i])
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

func (r *Request) deriveModes() {
	r.FixPermsOnly = r.FixPerms && !r.DoBackup && !r.DoPrune && !r.DoCleanupStore
}

func (r *Request) validateCombos() error {
	if r.ForcePrune && !r.DoPrune {
		return NewRequestError("--force-prune requires --prune")
	}
	if !r.DoBackup && !r.DoPrune && !r.DoCleanupStore && !r.FixPerms {
		return NewUsageRequestError("at least one operation is required: specify --backup, --prune, --cleanup-storage, or --fix-perms")
	}
	if r.RequestedTarget == "" {
		return NewRequestError("--target is required")
	}
	if err := ValidateTargetName(r.RequestedTarget); err != nil {
		return err
	}
	return nil
}

func parseTopLevelMetaRequest(args []string, meta Metadata, rt Runtime) *ParseResult {
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

func consumeSharedFlag(args []string, index *int, req *Request, opts sharedFlagOptions) (bool, error) {
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
		return "", NewUsageRequestError("%s requires a value", flag)
	}
	*index++
	return args[*index], nil
}

func parseSourcePositional(positional []string) (string, error) {
	if len(positional) < 1 {
		return "", NewUsageRequestError("source directory required")
	}
	if len(positional) > 1 {
		return "", NewUsageRequestError("unexpected extra arguments: %s", strings.Join(positional[1:], " "))
	}
	return positional[0], nil
}

func validateTargetAndLabel(req *Request) error {
	if req.RequestedTarget == "" {
		return NewRequestError("--target is required")
	}
	if err := ValidateTargetName(req.RequestedTarget); err != nil {
		return err
	}
	return validateLabel(req.Source)
}

func validateLabel(source string) error {
	if err := ValidateLabel(source); err != nil {
		return fmt.Errorf("Invalid source label: %w", err)
	}
	return nil
}

func isOption(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}
