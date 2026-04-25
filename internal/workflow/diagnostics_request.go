package workflow

// DiagnosticsRequest is the diagnostics command's narrowed view of CLI intent.
//
// The parser still returns Request while command-specific workflow models are
// introduced incrementally. Diagnostics code should use this type so it does
// not depend on unrelated restore, notify, update, health, or runtime fields.
type DiagnosticsRequest struct {
	Command     string
	Label       string
	TargetName  string
	ConfigDir   string
	SecretsDir  string
	JSONSummary bool
}

func NewDiagnosticsRequest(req *Request) DiagnosticsRequest {
	if req == nil {
		return DiagnosticsRequest{}
	}
	return DiagnosticsRequest{
		Command:     req.DiagnosticsCommand,
		Label:       req.Source,
		TargetName:  req.RequestedTarget,
		ConfigDir:   req.ConfigDir,
		SecretsDir:  req.SecretsDir,
		JSONSummary: req.JSONSummary,
	}
}

func (r DiagnosticsRequest) Target() string {
	return r.TargetName
}

func (r DiagnosticsRequest) legacyConfigRequest() *Request {
	return &Request{
		Source:          r.Label,
		ConfigDir:       r.ConfigDir,
		SecretsDir:      r.SecretsDir,
		RequestedTarget: r.Target(),
	}
}
