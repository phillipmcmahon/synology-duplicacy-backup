package command

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"
)

func parseHealthRequest(args []string, meta workflow.Metadata, rt workflow.Env) (*ParseResult, error) {
	if len(args) == 0 {
		return nil, workflowcore.NewUsageRequestError("health command required")
	}
	if result := parseHelpRequest(args, UsageText(meta, rt), FullUsageText(meta, rt)); result != nil {
		return result, nil
	}

	action := args[0]
	switch action {
	case "status", "doctor", "verify":
	default:
		return nil, workflowcore.NewUsageRequestError("unknown health command %s", action)
	}

	req, err := parseHealthFlags(args[1:])
	if err != nil {
		return nil, err
	}
	req.Command = "health"
	req.HealthCommand = action
	if err := validateTargetAndLabel(req); err != nil {
		return nil, err
	}

	return &ParseResult{Request: req}, nil
}

func parseHealthFlags(args []string) (*workflowcore.Request, error) {
	req := &workflowcore.Request{}
	return req, parseSourceFlags(args, req, sharedFlagOptions{
		target:      true,
		verbose:     true,
		jsonSummary: true,
		configDir:   true,
		secretsDir:  true,
	}, nil)
}
