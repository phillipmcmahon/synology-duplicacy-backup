package health

import "github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"

// HealthRequest is the health command's narrowed view of CLI intent.
type HealthRequest struct {
	Command     string
	Label       string
	TargetName  string
	ConfigDir   string
	SecretsDir  string
	JSONSummary bool
	Verbose     bool
}

func NewHealthRequest(req *Request) HealthRequest {
	if req == nil {
		return HealthRequest{}
	}
	return HealthRequest{
		Command:     req.HealthCommand,
		Label:       req.Source,
		TargetName:  req.Target(),
		ConfigDir:   req.ConfigDir,
		SecretsDir:  req.SecretsDir,
		JSONSummary: req.JSONSummary,
		Verbose:     req.Verbose,
	}
}

func (r HealthRequest) Target() string {
	return r.TargetName
}

func (r HealthRequest) PlanRequest() workflow.ConfigPlanRequest {
	return workflow.ConfigPlanRequest{
		Label:      r.Label,
		TargetName: r.Target(),
		ConfigDir:  r.ConfigDir,
		SecretsDir: r.SecretsDir,
	}
}
