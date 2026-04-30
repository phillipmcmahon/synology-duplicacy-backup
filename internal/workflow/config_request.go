package workflow

// ConfigRequest is the config command's narrowed view of CLI intent.
type ConfigRequest struct {
	Command    string
	Label      string
	TargetName string
	ConfigDir  string
	SecretsDir string
}

func NewConfigRequest(req *Request) ConfigRequest {
	if req == nil {
		return ConfigRequest{}
	}
	return ConfigRequest{
		Command:    req.ConfigCommand,
		Label:      req.Source,
		TargetName: req.Target(),
		ConfigDir:  req.ConfigDir,
		SecretsDir: req.SecretsDir,
	}
}

func (r ConfigRequest) Target() string {
	return r.TargetName
}

func (r ConfigRequest) PlanRequest() ConfigPlanRequest {
	return ConfigPlanRequest{
		Label:      r.Label,
		TargetName: r.Target(),
		ConfigDir:  r.ConfigDir,
		SecretsDir: r.SecretsDir,
	}
}
