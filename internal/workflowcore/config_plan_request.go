package workflowcore

// ConfigPlanRequest is the planner's narrow input for resolving config and
// secrets paths for one label/target pair.
type ConfigPlanRequest struct {
	Label      string
	TargetName string
	ConfigDir  string
	SecretsDir string
}

func NewConfigPlanRequest(req *Request) ConfigPlanRequest {
	if req == nil {
		return ConfigPlanRequest{}
	}
	return ConfigPlanRequest{
		Label:      req.Source,
		TargetName: req.Target(),
		ConfigDir:  req.ConfigDir,
		SecretsDir: req.SecretsDir,
	}
}

func (r ConfigPlanRequest) Target() string {
	return r.TargetName
}
