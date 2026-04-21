package workflow

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
	ConfigCommand      string
	HealthCommand      string
	NotifyCommand      string
	RestoreCommand     string
	UpdateCommand      string
	FixPerms           bool
	ForcePrune         bool
	RequestedTarget    string
	DryRun             bool
	Verbose            bool
	JSONSummary        bool
	ConfigDir          string
	SecretsDir         string
	Source             string
	NotifyProvider     string
	NotifySeverity     string
	NotifySummary      string
	NotifyMessage      string
	NotifyScope        string
	NotifyEvent        string
	UpdateVersion      string
	UpdateKeep         int
	UpdateAttestations string
	UpdateCheckOnly    bool
	UpdateYes          bool
	UpdateForce        bool
	DoBackup           bool
	DoPrune            bool
	DoCleanupStore     bool
	FixPermsOnly       bool
	DefaultNotice      string
}

func (r *Request) Target() string {
	if r != nil {
		return r.RequestedTarget
	}
	return ""
}

func (r *Request) DeriveModes() {
	r.FixPermsOnly = r.FixPerms && !r.DoBackup && !r.DoPrune && !r.DoCleanupStore
}

func (r *Request) ValidateCombos() error {
	if r.ForcePrune && !r.DoPrune {
		return NewRequestError("--force-prune requires --prune")
	}
	if !r.DoBackup && !r.DoPrune && !r.DoCleanupStore && !r.FixPerms {
		return NewUsageRequestError("at least one operation is required: specify --backup, --prune, --cleanup-storage, or --fix-perms")
	}
	if r.RequestedTarget == "" {
		return NewRequestError("--target is required")
	}
	if err := ValidateTargetName(r.RequestedTarget); err != nil {
		return err
	}
	return nil
}
