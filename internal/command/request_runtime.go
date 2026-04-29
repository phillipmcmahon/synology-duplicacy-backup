package command

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"
)

func parseRuntimeCommandRequest(command string, args []string, meta workflow.Metadata, rt workflow.Env) (*ParseResult, error) {
	if result := parseHelpRequest(args, UsageText(meta, rt), FullUsageText(meta, rt)); result != nil {
		return result, nil
	}
	req, err := parseRuntimeCommandFlags(command, args)
	if err != nil {
		return nil, err
	}
	switch command {
	case "backup":
		req.Command = "backup"
		req.DoBackup = true
	case "prune":
		req.Command = "prune"
		req.DoPrune = true
	case "cleanup-storage":
		req.Command = "cleanup-storage"
		req.DoCleanupStore = true
	default:
		return nil, workflowcore.NewUsageRequestError("unknown command %s", command)
	}
	if err := validateTargetAndLabel(req); err != nil {
		return nil, err
	}

	return &ParseResult{Request: req}, nil
}

func parseRuntimeCommandFlags(command string, args []string) (*workflowcore.Request, error) {
	req := &workflowcore.Request{}
	err := parseSourceFlags(args, req, sharedFlagOptions{
		target:      true,
		dryRun:      true,
		verbose:     true,
		jsonSummary: true,
		configDir:   true,
		secretsDir:  true,
	}, func(args []string, index *int, req *workflowcore.Request) (bool, error) {
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
