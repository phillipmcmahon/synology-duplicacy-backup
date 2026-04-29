package command

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"
)

func parseRestoreRequest(args []string, meta workflow.Metadata, rt workflow.Env) (*ParseResult, error) {
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
		return nil, workflowcore.NewUsageRequestError("unknown restore command %s", action)
	}

	req, err := parseRestoreFlags(action, args[1:])
	if err != nil {
		return nil, err
	}
	req.Command = "restore"
	req.RestoreCommand = action
	switch action {
	case "run":
		if req.RestoreRevision <= 0 {
			return nil, workflowcore.NewUsageRequestError("restore %s requires --revision <id>", action)
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

func parseRestoreFlags(action string, args []string) (*workflowcore.Request, error) {
	req := &workflowcore.Request{}
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
	}, func(args []string, index *int, req *workflowcore.Request) (bool, error) {
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
