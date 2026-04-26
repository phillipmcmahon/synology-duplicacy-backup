package workflow

type RuntimeMode string

const (
	RuntimeModeBackup         RuntimeMode = "backup"
	RuntimeModePrune          RuntimeMode = "prune"
	RuntimeModeCleanupStorage RuntimeMode = "cleanup-storage"
)

// RuntimeRequest is the runtime command's narrowed view of CLI intent.
type RuntimeRequest struct {
	Mode          RuntimeMode
	Label         string
	TargetName    string
	ConfigDir     string
	SecretsDir    string
	DryRun        bool
	Verbose       bool
	JSONSummary   bool
	ForcePrune    bool
	DefaultNotice string
}

func NewRuntimeRequest(req *Request) RuntimeRequest {
	if req == nil {
		return RuntimeRequest{}
	}
	mode := RuntimeMode("")
	switch {
	case req.DoBackup:
		mode = RuntimeModeBackup
	case req.DoPrune:
		mode = RuntimeModePrune
	case req.DoCleanupStore:
		mode = RuntimeModeCleanupStorage
	}
	return RuntimeRequest{
		Mode:          mode,
		Label:         req.Source,
		TargetName:    req.RequestedTarget,
		ConfigDir:     req.ConfigDir,
		SecretsDir:    req.SecretsDir,
		DryRun:        req.DryRun,
		Verbose:       req.Verbose,
		JSONSummary:   req.JSONSummary,
		ForcePrune:    req.ForcePrune,
		DefaultNotice: req.DefaultNotice,
	}
}

func (r RuntimeRequest) Target() string {
	return r.TargetName
}

func (r RuntimeRequest) DoBackup() bool {
	return r.Mode == RuntimeModeBackup
}

func (r RuntimeRequest) DoPrune() bool {
	return r.Mode == RuntimeModePrune
}

func (r RuntimeRequest) DoCleanupStore() bool {
	return r.Mode == RuntimeModeCleanupStorage
}
