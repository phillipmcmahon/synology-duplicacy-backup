package presentation

const (
	LabelBackupState       = "Backup State"
	LabelBackupFreshness   = "Backup Freshness"
	LabelBtrfs             = "Btrfs"
	LabelBtrfsRoot         = "Btrfs Root"
	LabelBtrfsSource       = "Btrfs Source"
	LabelConfigFile        = "Config File"
	LabelIntegrityCheck    = "Integrity Check"
	LabelLastDoctorRun     = "Last Doctor Run"
	LabelLastVerifyRun     = "Last Verify Run"
	LabelLatestRevision    = "Latest Revision"
	LabelRepository        = "Repository"
	LabelRepositoryAccess  = "Repository Access"
	LabelRevisionCount     = "Revision Count"
	LabelRevisionsChecked  = "Revisions Checked"
	LabelRevisionsFailed   = "Revisions Failed"
	LabelRevisionsPassed   = "Revisions Passed"
	LabelRootConfigProfile = "Root Config Profile"
	LabelSourcePath        = "Source Path"
	LabelStorageAccess     = "Storage Access"
)

const (
	ValueDegraded       = "Degraded"
	ValueFailed         = "Failed"
	ValueHealthy        = "Healthy"
	ValueInvalid        = "Invalid"
	ValueLimited        = "Limited"
	ValueNotChecked     = "Not checked"
	ValueNotConfigured  = "Not configured"
	ValueNotEnabled     = "Not enabled"
	ValueNotInitialized = "Not initialized"
	ValueNotRequired    = "Not required"
	ValueParsed         = "Parsed"
	ValuePassed         = "Passed"
	ValuePresent        = "Present"
	ValueReadable       = "Readable"
	ValueRequiresSudo   = "Requires sudo"
	ValueResolved       = "Resolved"
	ValueSkipped        = "Skipped"
	ValueUnhealthy      = "Unhealthy"
	ValueValidated      = "Validated"
	ValueValid          = "Valid"
	ValueWritable       = "Writable"
)

const localRepositorySudoDetail = "local filesystem repository storage is protected by OS filesystem permissions"

func LocalRepositoryRequiresSudoMessage(command string) string {
	message := localRepositorySudoDetail + "; rerun with sudo from the operator account"
	if command == "" {
		return ValueRequiresSudo + "; " + message
	}
	return command + " requires sudo; " + message
}

// displayVocabulary maps internal status/check keys and shared semantic values
// to the operator-facing vocabulary used across plain reports and live runtime
// output. Identity mappings are deliberately explicit so a future wording
// change to a constant cannot be bypassed by DisplayLabel's fallback branch.
var displayVocabulary = map[string]string{
	"Backup freshness":    LabelBackupFreshness,
	"Backup Freshness":    LabelBackupFreshness,
	"Backup state":        LabelBackupState,
	"Backup State":        LabelBackupState,
	"Btrfs":               LabelBtrfs,
	"Btrfs root":          LabelBtrfsRoot,
	"Btrfs Root":          LabelBtrfsRoot,
	"Btrfs source":        LabelBtrfsSource,
	"Btrfs Source":        LabelBtrfsSource,
	"Config file":         LabelConfigFile,
	"Config File":         LabelConfigFile,
	"Integrity check":     LabelIntegrityCheck,
	"Integrity Check":     LabelIntegrityCheck,
	"Last doctor run":     LabelLastDoctorRun,
	"Last Doctor Run":     LabelLastDoctorRun,
	"Last verify run":     LabelLastVerifyRun,
	"Last Verify Run":     LabelLastVerifyRun,
	"Latest revision":     LabelLatestRevision,
	"Latest Revision":     LabelLatestRevision,
	"Repository":          LabelRepository,
	"Repository access":   LabelRepositoryAccess,
	"Repository Access":   LabelRepositoryAccess,
	"Revision count":      LabelRevisionCount,
	"Revision Count":      LabelRevisionCount,
	"Revisions checked":   LabelRevisionsChecked,
	"Revisions Checked":   LabelRevisionsChecked,
	"Revisions failed":    LabelRevisionsFailed,
	"Revisions Failed":    LabelRevisionsFailed,
	"Revisions passed":    LabelRevisionsPassed,
	"Revisions Passed":    LabelRevisionsPassed,
	"Root config profile": LabelRootConfigProfile,
	"Root Config Profile": LabelRootConfigProfile,
	"Source path":         LabelSourcePath,
	"Source Path":         LabelSourcePath,
	"Storage access":      LabelStorageAccess,
	"Storage Access":      LabelStorageAccess,
	"Degraded":            ValueDegraded,
	"Failed":              ValueFailed,
	"Healthy":             ValueHealthy,
	"Invalid":             ValueInvalid,
	"Limited":             ValueLimited,
	"Not checked":         ValueNotChecked,
	"Not configured":      ValueNotConfigured,
	"Not enabled":         ValueNotEnabled,
	"Not initialized":     ValueNotInitialized,
	"Not required":        ValueNotRequired,
	"Parsed":              ValueParsed,
	"Passed":              ValuePassed,
	"Present":             ValuePresent,
	"Readable":            ValueReadable,
	"Requires sudo":       ValueRequiresSudo,
	"Resolved":            ValueResolved,
	"Skipped":             ValueSkipped,
	"Unhealthy":           ValueUnhealthy,
	"Validated":           ValueValidated,
	"Valid":               ValueValid,
	"Writable":            ValueWritable,
}

// DisplayLabel maps internal status/check keys to the operator-facing
// vocabulary used across plain reports and live runtime output.
func DisplayLabel(name string) string {
	if label, ok := displayVocabulary[name]; ok {
		return label
	}
	return name
}
