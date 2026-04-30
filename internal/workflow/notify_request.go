package workflow

import "github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"

// NotifyRequest is the notify command's narrowed view of CLI intent.
//
// The parser still returns Request while command-specific workflow models are
// introduced incrementally. Notify code should use this type so it does not
// depend on unrelated restore, update, health, or runtime fields.
type NotifyRequest struct {
	Command     string
	Scope       string
	Label       string
	TargetName  string
	ConfigDir   string
	SecretsDir  string
	JSONSummary bool
	DryRun      bool
	Provider    string
	Event       notify.EventID
	Severity    string
	Summary     string
	Message     string
}

func NewNotifyRequest(req *Request) NotifyRequest {
	if req == nil {
		return NotifyRequest{}
	}
	return NotifyRequest{
		Command:     req.NotifyCommand,
		Scope:       req.NotifyScope,
		Label:       req.Source,
		TargetName:  req.Target(),
		ConfigDir:   req.ConfigDir,
		SecretsDir:  req.SecretsDir,
		JSONSummary: req.JSONSummary,
		DryRun:      req.DryRun,
		Provider:    req.NotifyProvider,
		Event:       notify.EventID(req.NotifyEvent),
		Severity:    req.NotifySeverity,
		Summary:     req.NotifySummary,
		Message:     req.NotifyMessage,
	}
}

func (r NotifyRequest) Target() string {
	return r.TargetName
}

func (r NotifyRequest) PlanRequest() ConfigPlanRequest {
	return ConfigPlanRequest{
		Label:      r.Label,
		TargetName: r.Target(),
		ConfigDir:  r.ConfigDir,
		SecretsDir: r.SecretsDir,
	}
}
