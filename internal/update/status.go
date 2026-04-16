package update

type Status string

const (
	StatusUnknown            Status = ""
	StatusInstalled          Status = "installed"
	StatusCurrent            Status = "current"
	StatusAvailable          Status = "available"
	StatusReinstallRequested Status = "reinstall-requested"
	StatusFailed             Status = "failed"
	StatusCancelled          Status = "cancelled"
)

type Options struct {
	RequestedVersion string
	CheckOnly        bool
	Force            bool
	Yes              bool
	Keep             int
}
