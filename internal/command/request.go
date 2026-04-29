package command

import (
	"fmt"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"
)

type ParseResult struct {
	Command Command
	Request *workflowcore.Request
	Output  string
	Handled bool
}

func ParseRequest(args []string, meta workflow.Metadata, rt workflow.Env) (*ParseResult, error) {
	if len(args) == 0 {
		return &ParseResult{Handled: true, Output: UsageText(meta, rt)}, nil
	}

	if spec, ok := commandSpec(args[0]); ok {
		result, err := spec.parse(args[1:], meta, rt)
		if err != nil || result == nil || result.Handled || result.Request == nil {
			return result, err
		}
		result.Command = newParsedCommand(spec, result.Request)
		return result, nil
	}

	if result := parseTopLevelMetaRequest(args, meta, rt); result != nil {
		return result, nil
	}

	if isOption(args[0]) {
		return nil, workflowcore.NewUsageRequestError("unknown top-level option %s; use a command such as backup, prune, or cleanup-storage", args[0])
	}
	return nil, workflowcore.NewUsageRequestError("unknown command %s", args[0])
}

func parseSourceFlagsWithLabel(args []string, req *workflowcore.Request, opts sharedFlagOptions, extra extraFlagParser) error {
	if err := parseSourceFlags(args, req, opts, extra); err != nil {
		if strings.Contains(err.Error(), "source directory required") {
			return workflowcore.NewUsageRequestError("backup label required")
		}
		return err
	}
	return nil
}

type extraFlagParser func(args []string, index *int, req *workflowcore.Request) (bool, error)

func parseSourceFlags(args []string, req *workflowcore.Request, opts sharedFlagOptions, extra extraFlagParser) error {
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
			return workflowcore.NewUsageRequestError("unknown option %s", args[i])
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

func parseTopLevelMetaRequest(args []string, meta workflow.Metadata, rt workflow.Env) *ParseResult {
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

func consumeSharedFlag(args []string, index *int, req *workflowcore.Request, opts sharedFlagOptions) (bool, error) {
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
		return "", workflowcore.NewUsageRequestError("%s requires a value", flag)
	}
	*index++
	return args[*index], nil
}

func parseSourcePositional(positional []string) (string, error) {
	if len(positional) < 1 {
		return "", workflowcore.NewUsageRequestError("source directory required")
	}
	if len(positional) > 1 {
		return "", workflowcore.NewUsageRequestError("unexpected extra arguments: %s", strings.Join(positional[1:], " "))
	}
	return positional[0], nil
}

func parseNonNegativeInt(value string, flag string) (int, error) {
	var parsed int
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, workflowcore.NewUsageRequestError("%s must be a non-negative integer", flag)
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
		return 0, workflowcore.NewUsageRequestError("%s must be a positive integer", flag)
	}
	return parsed, nil
}

func validateTargetAndLabel(req *workflowcore.Request) error {
	if req.RequestedTarget == "" {
		return workflowcore.NewRequestError("--target is required")
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

func isOption(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}
