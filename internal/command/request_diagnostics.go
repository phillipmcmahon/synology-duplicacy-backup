package command

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"
)

func parseDiagnosticsRequest(args []string, meta workflow.Metadata, rt workflow.Env) (*ParseResult, error) {
	if result := parseHelpRequest(args, DiagnosticsUsageText(meta, rt), FullDiagnosticsUsageText(meta, rt)); result != nil {
		return result, nil
	}
	req, err := parseDiagnosticsFlags(args)
	if err != nil {
		return nil, err
	}
	req.Command = "diagnostics"
	req.DiagnosticsCommand = "diagnostics"
	if err := validateTargetAndLabel(req); err != nil {
		return nil, err
	}

	return &ParseResult{Request: req}, nil
}

func parseDiagnosticsFlags(args []string) (*workflowcore.Request, error) {
	req := &workflowcore.Request{}
	return req, parseSourceFlags(args, req, sharedFlagOptions{
		target:      true,
		jsonSummary: true,
		configDir:   true,
		secretsDir:  true,
	}, nil)
}
