package workflow

import (
	"fmt"
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
	Mode       string
	FixPerms   bool
	ForcePrune bool
	RemoteMode bool
	DryRun     bool
	ConfigDir  string
	SecretsDir string
	Source     string

	DoBackup      bool
	DoPrune       bool
	DeepPruneMode bool
	FixPermsOnly  bool
	DefaultNotice string
}

type ParseResult struct {
	Request *Request
	Output  string
	Handled bool
}

func ParseRequest(args []string, meta Metadata, rt Runtime) (*ParseResult, error) {
	for _, arg := range args {
		if arg == "--help" {
			return &ParseResult{Handled: true, Output: UsageText(meta, rt)}, nil
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

func parseFlags(args []string) (*Request, error) {
	req := &Request{}
	var positional []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--backup", "--prune", "--prune-deep":
			if req.Mode != "" {
				return nil, NewUsageRequestError("only one mode may be specified: --backup, --prune, or --prune-deep")
			}
			req.Mode = args[i][2:]
		case "--fix-perms":
			req.FixPerms = true
		case "--force-prune":
			req.ForcePrune = true
		case "--remote":
			req.RemoteMode = true
		case "--dry-run":
			req.DryRun = true
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

	if req.Mode == "" && !req.FixPerms {
		req.Mode = "backup"
		req.DefaultNotice = "No mode specified: defaulting to backup only."
	}
	if req.Mode == "" && req.FixPerms {
		req.DefaultNotice = "No primary mode specified: using fix-perms only."
	}

	if len(positional) < 1 {
		return nil, NewUsageRequestError("source directory required")
	}
	req.Source = positional[0]
	return req, nil
}

func (r *Request) deriveModes() {
	r.DoBackup = r.Mode == "backup"
	r.DoPrune = r.Mode == "prune" || r.Mode == "prune-deep"
	r.DeepPruneMode = r.Mode == "prune-deep"
	r.FixPermsOnly = r.FixPerms && !r.DoBackup && !r.DoPrune
}

func (r *Request) validateCombos() error {
	if r.DeepPruneMode && !r.ForcePrune {
		return NewRequestError("--prune-deep requires --force-prune")
	}
	if r.ForcePrune && !r.DoPrune {
		return NewRequestError("--force-prune requires --prune or --prune-deep")
	}
	if r.FixPerms && r.RemoteMode {
		return NewRequestError("--fix-perms is only valid for local backups; cannot be used with --remote")
	}
	return nil
}
