package command

import (
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func parseRollbackRequest(args []string, meta workflow.Metadata, rt workflow.Env) (*ParseResult, error) {
	if result := parseHelpRequest(args, RollbackUsageText(meta, rt), FullRollbackUsageText(meta, rt)); result != nil {
		return result, nil
	}
	req, err := parseRollbackFlags(args)
	if err != nil {
		return nil, err
	}
	req.Command = "rollback"
	req.RollbackCommand = "rollback"
	return &ParseResult{Request: req}, nil
}

func parseUpdateRequest(args []string, meta workflow.Metadata, rt workflow.Env) (*ParseResult, error) {
	if result := parseHelpRequest(args, UpdateUsageText(meta, rt), FullUpdateUsageText(meta, rt)); result != nil {
		return result, nil
	}

	req, err := parseUpdateFlags(args)
	if err != nil {
		return nil, err
	}
	req.Command = "update"
	req.UpdateCommand = "update"
	return &ParseResult{Request: req}, nil
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
