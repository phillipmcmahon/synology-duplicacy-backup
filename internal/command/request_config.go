package command

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"
)

func parseConfigRequest(args []string, meta workflow.Metadata, rt workflow.Env) (*ParseResult, error) {
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
		return nil, workflowcore.NewUsageRequestError("unknown config command %s", action)
	}

	req, err := parseConfigFlags(args[1:])
	if err != nil {
		return nil, err
	}
	req.Command = "config"
	req.ConfigCommand = action
	if err := validateTargetAndLabel(req); err != nil {
		return nil, err
	}

	return &ParseResult{Request: req}, nil
}

func parseConfigFlags(args []string) (*workflowcore.Request, error) {
	req := &workflowcore.Request{}
	return req, parseSourceFlags(args, req, sharedFlagOptions{
		target:     true,
		verbose:    true,
		configDir:  true,
		secretsDir: true,
	}, nil)
}
