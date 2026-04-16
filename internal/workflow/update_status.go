package workflow

type UpdateStatus string

const (
	UpdateStatusUnknown            UpdateStatus = ""
	UpdateStatusInstalled          UpdateStatus = "installed"
	UpdateStatusCurrent            UpdateStatus = "current"
	UpdateStatusAvailable          UpdateStatus = "available"
	UpdateStatusReinstallRequested UpdateStatus = "reinstall-requested"
	UpdateStatusFailed             UpdateStatus = "failed"
	UpdateStatusCancelled          UpdateStatus = "cancelled"
)
