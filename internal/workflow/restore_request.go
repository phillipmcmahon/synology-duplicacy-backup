package workflow

import "strings"

const (
	RestoreCommandPlan          = "plan"
	RestoreCommandListRevisions = "list-revisions"
	RestoreCommandRun           = "run"
	RestoreCommandSelect        = "select"
)

// RestoreRequest is the restore command's narrowed view of CLI intent.
//
// The parser still returns Request while the broader request-model refactor is
// incremental. Restore handlers should use this type so new restore behaviour
// does not keep depending on unrelated backup, notify, update, or health flags.
type RestoreRequest struct {
	Command           string
	Label             string
	TargetName        string
	ConfigDir         string
	SecretsDir        string
	JSONSummary       bool
	DryRun            bool
	Workspace         string
	WorkspaceRoot     string
	WorkspaceTemplate string
	Revision          int
	Path              string
	PathPrefix        string
	Limit             int
	Yes               bool
}

func NewRestoreRequest(req *Request) RestoreRequest {
	if req == nil {
		// Keep projectors mechanical and tolerant; command validation rejects
		// the resulting empty request at the boundary.
		return RestoreRequest{}
	}
	return RestoreRequest{
		Command:           req.RestoreCommand,
		Label:             req.Source,
		TargetName:        strings.TrimSpace(req.RequestedTarget),
		ConfigDir:         req.ConfigDir,
		SecretsDir:        req.SecretsDir,
		JSONSummary:       req.JSONSummary,
		DryRun:            req.DryRun,
		Workspace:         req.RestoreWorkspace,
		WorkspaceRoot:     req.RestoreWorkspaceRoot,
		WorkspaceTemplate: req.RestoreWorkspaceTemplate,
		Revision:          req.RestoreRevision,
		Path:              req.RestorePath,
		PathPrefix:        req.RestorePathPrefix,
		Limit:             req.RestoreLimit,
		Yes:               req.RestoreYes,
	}
}

func (r RestoreRequest) Target() string {
	return r.TargetName
}

func (r RestoreRequest) UsesProgress() bool {
	return r.Command == RestoreCommandRun || r.Command == RestoreCommandSelect
}

func (r RestoreRequest) PlanRequest() ConfigPlanRequest {
	return ConfigPlanRequest{
		Label:      r.Label,
		TargetName: r.Target(),
		ConfigDir:  r.ConfigDir,
		SecretsDir: r.SecretsDir,
	}
}
