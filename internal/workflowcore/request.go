package workflowcore

import "fmt"

// RequestError describes a request-parsing or request-validation failure.
// ShowUsage marks errors that should be followed by usage text.
type RequestError struct {
	message   string
	ShowUsage bool
}

func (e *RequestError) Error() string {
	return e.message
}

func NewRequestError(format string, args ...interface{}) *RequestError {
	return &RequestError{message: fmt.Sprintf(format, args...)}
}

func NewUsageRequestError(format string, args ...interface{}) *RequestError {
	return &RequestError{message: fmt.Sprintf(format, args...), ShowUsage: true}
}

type Request struct {
	Command                  string
	ConfigCommand            string
	DiagnosticsCommand       string
	HealthCommand            string
	NotifyCommand            string
	RestoreCommand           string
	RestoreWorkspace         string
	RestoreWorkspaceRoot     string
	RestoreWorkspaceTemplate string
	RestoreRevision          int
	RestorePath              string
	RestorePathPrefix        string
	RestoreLimit             int
	RestoreYes               bool
	RollbackCommand          string
	RollbackVersion          string
	RollbackCheckOnly        bool
	RollbackYes              bool
	UpdateCommand            string
	ForcePrune               bool
	RequestedTarget          string
	DryRun                   bool
	Verbose                  bool
	JSONSummary              bool
	ConfigDir                string
	SecretsDir               string
	Source                   string
	NotifyProvider           string
	NotifySeverity           string
	NotifySummary            string
	NotifyMessage            string
	NotifyScope              string
	NotifyEvent              string
	UpdateVersion            string
	UpdateKeep               int
	UpdateAttestations       string
	UpdateCheckOnly          bool
	UpdateYes                bool
	UpdateForce              bool
	DoBackup                 bool
	DoPrune                  bool
	DoCleanupStore           bool
	DefaultNotice            string
}

func (r *Request) Target() string {
	if r != nil {
		return r.RequestedTarget
	}
	return ""
}
