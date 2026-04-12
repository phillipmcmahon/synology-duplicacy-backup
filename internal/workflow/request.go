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
	FixPerms        bool
	ForcePrune      bool
	RequestedTarget string
	DryRun          bool
	Verbose         bool
	JSONSummary     bool
	ConfigDir       string
	SecretsDir      string
	Source          string
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

	for _, arg := range args {
		if arg == "--help" {
			return &ParseResult{Handled: true, Output: UsageText(meta, rt)}, nil
		}
		if arg == "--help-full" {
			return &ParseResult{Handled: true, Output: FullUsageText(meta, rt)}, nil
		}
		if arg == "--version" || arg == "-v" {
			return &ParseResult{Handled: true, Output: VersionText(meta)}, nil
		}
	}

	req, err := parseFlags(args)
	if err != nil {
		return nil, err
	}

	req.deriveModes()

	if err := req.validateCombos(); err != nil {
		return nil, err
	}
	if err := ValidateLabel(req.Source); err != nil {
		return nil, fmt.Errorf("Invalid source label: %w", err)
	}

	return &ParseResult{Request: req}, nil
}

func parseHealthRequest(args []string, meta Metadata, rt Runtime) (*ParseResult, error) {
	if len(args) == 0 {
		return nil, NewUsageRequestError("health command required")
	}
	for _, arg := range args {
		if arg == "--help" {
			return &ParseResult{Handled: true, Output: UsageText(meta, rt)}, nil
		}
		if arg == "--help-full" {
			return &ParseResult{Handled: true, Output: FullUsageText(meta, rt)}, nil
		}
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
	if req.RequestedTarget == "" {
		return nil, NewRequestError("--target is required")
	}
	if err := ValidateTargetName(req.RequestedTarget); err != nil {
		return nil, err
	}

	if err := ValidateLabel(req.Source); err != nil {
		return nil, fmt.Errorf("Invalid source label: %w", err)
	}

	return &ParseResult{Request: req}, nil
}

func parseConfigRequest(args []string, meta Metadata, rt Runtime) (*ParseResult, error) {
	if len(args) == 0 {
		return &ParseResult{Handled: true, Output: ConfigUsageText(meta, rt)}, nil
	}
	for _, arg := range args {
		if arg == "--help" {
			return &ParseResult{Handled: true, Output: ConfigUsageText(meta, rt)}, nil
		}
		if arg == "--help-full" {
			return &ParseResult{Handled: true, Output: FullConfigUsageText(meta, rt)}, nil
		}
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
	if req.RequestedTarget == "" {
		return nil, NewRequestError("--target is required")
	}
	if err := ValidateTargetName(req.RequestedTarget); err != nil {
		return nil, err
	}

	if err := ValidateLabel(req.Source); err != nil {
		return nil, fmt.Errorf("Invalid source label: %w", err)
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
		case "--target":
			if i+1 >= len(args) {
				return nil, NewUsageRequestError("--target requires a value")
			}
			i++
			req.RequestedTarget = args[i]
		case "--dry-run":
			req.DryRun = true
		case "--verbose":
			req.Verbose = true
		case "--json-summary":
			req.JSONSummary = true
		case "--config-dir":
			if i+1 >= len(args) {
				return nil, NewUsageRequestError("--config-dir requires a value")
			}
			i++
			req.ConfigDir = args[i]
		case "--secrets-dir":
			if i+1 >= len(args) {
				return nil, NewUsageRequestError("--secrets-dir requires a value")
			}
			i++
			req.SecretsDir = args[i]
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return nil, NewUsageRequestError("unknown option %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	if len(positional) < 1 {
		return nil, NewUsageRequestError("source directory required")
	}
	if len(positional) > 1 {
		return nil, NewUsageRequestError("unexpected extra arguments: %s", strings.Join(positional[1:], " "))
	}
	req.Source = positional[0]
	return req, nil
}

func parseConfigFlags(args []string) (*Request, error) {
	req := &Request{}
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--target":
			if i+1 >= len(args) {
				return nil, NewUsageRequestError("--target requires a value")
			}
			i++
			req.RequestedTarget = args[i]
		case "--config-dir":
			if i+1 >= len(args) {
				return nil, NewUsageRequestError("--config-dir requires a value")
			}
			i++
			req.ConfigDir = args[i]
		case "--secrets-dir":
			if i+1 >= len(args) {
				return nil, NewUsageRequestError("--secrets-dir requires a value")
			}
			i++
			req.SecretsDir = args[i]
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return nil, NewUsageRequestError("unknown option %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	if len(positional) < 1 {
		return nil, NewUsageRequestError("source directory required")
	}
	if len(positional) > 1 {
		return nil, NewUsageRequestError("unexpected extra arguments: %s", strings.Join(positional[1:], " "))
	}
	req.Source = positional[0]
	return req, nil
}

func parseHealthFlags(args []string) (*Request, error) {
	req := &Request{}
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--target":
			if i+1 >= len(args) {
				return nil, NewUsageRequestError("--target requires a value")
			}
			i++
			req.RequestedTarget = args[i]
		case "--verbose":
			req.Verbose = true
		case "--json-summary":
			req.JSONSummary = true
		case "--config-dir":
			if i+1 >= len(args) {
				return nil, NewUsageRequestError("--config-dir requires a value")
			}
			i++
			req.ConfigDir = args[i]
		case "--secrets-dir":
			if i+1 >= len(args) {
				return nil, NewUsageRequestError("--secrets-dir requires a value")
			}
			i++
			req.SecretsDir = args[i]
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return nil, NewUsageRequestError("unknown option %s", args[i])
			}
			positional = append(positional, args[i])
		}
	}

	if len(positional) < 1 {
		return nil, NewUsageRequestError("source directory required")
	}
	if len(positional) > 1 {
		return nil, NewUsageRequestError("unexpected extra arguments: %s", strings.Join(positional[1:], " "))
	}
	req.Source = positional[0]
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
