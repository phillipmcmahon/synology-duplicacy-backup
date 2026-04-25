package workflow

import "strings"

// RestoreRequest is the restore command's narrowed view of CLI intent.
//
// The parser still returns Request while the broader request-model refactor is
// incremental. Restore handlers should use this type so new restore behaviour
// does not keep depending on unrelated backup, notify, update, or health flags.
type RestoreRequest struct {
	Command     string
	Label       string
	TargetName  string
	ConfigDir   string
	SecretsDir  string
	JSONSummary bool
	DryRun      bool
	Workspace   string
	Revision    int
	Path        string
	PathPrefix  string
	Limit       int
	Yes         bool
}

func NewRestoreRequest(req *Request) RestoreRequest {
	if req == nil {
		return RestoreRequest{}
	}
	return RestoreRequest{
		Command:     req.RestoreCommand,
		Label:       req.Source,
		TargetName:  req.RequestedTarget,
		ConfigDir:   req.ConfigDir,
		SecretsDir:  req.SecretsDir,
		JSONSummary: req.JSONSummary,
		DryRun:      req.DryRun,
		Workspace:   req.RestoreWorkspace,
		Revision:    req.RestoreRevision,
		Path:        req.RestorePath,
		PathPrefix:  req.RestorePathPrefix,
		Limit:       req.RestoreLimit,
		Yes:         req.RestoreYes,
	}
}

func (r RestoreRequest) Target() string {
	return strings.TrimSpace(r.TargetName)
}

func (r RestoreRequest) ConfigRequest() *Request {
	return &Request{
		Source:          r.Label,
		ConfigDir:       r.ConfigDir,
		SecretsDir:      r.SecretsDir,
		RequestedTarget: r.Target(),
		DoBackup:        false,
		DoPrune:         false,
		DoCleanupStore:  false,
		FixPerms:        false,
	}
}
