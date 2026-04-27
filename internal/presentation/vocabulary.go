package presentation

const (
	LabelBackupFreshness   = "Backup Freshness"
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
	ValueNotChecked   = "Not checked"
	ValueNotRequired  = "Not required"
	ValueRequiresSudo = "Requires sudo"
	ValueValidated    = "Validated"
	ValueValid        = "Valid"
	ValueWritable     = "Writable"
)

// DisplayLabel maps internal status/check keys to the operator-facing label
// vocabulary used across plain reports and live runtime output.
func DisplayLabel(name string) string {
	switch name {
	case "Backup freshness":
		return LabelBackupFreshness
	case "Btrfs root":
		return LabelBtrfsRoot
	case "Btrfs source":
		return LabelBtrfsSource
	case "Config file":
		return LabelConfigFile
	case "Integrity check":
		return LabelIntegrityCheck
	case "Last doctor run":
		return LabelLastDoctorRun
	case "Last verify run":
		return LabelLastVerifyRun
	case "Latest revision":
		return LabelLatestRevision
	case "Repository access":
		return LabelRepositoryAccess
	case "Revision count":
		return LabelRevisionCount
	case "Revisions checked":
		return LabelRevisionsChecked
	case "Revisions failed":
		return LabelRevisionsFailed
	case "Revisions passed":
		return LabelRevisionsPassed
	case "Root config profile":
		return LabelRootConfigProfile
	case "Source path":
		return LabelSourcePath
	default:
		return name
	}
}
