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
	Attestations     string
}

type AttestationMode string

const (
	AttestationOff      AttestationMode = "off"
	AttestationAuto     AttestationMode = "auto"
	AttestationRequired AttestationMode = "required"
)

func normalizeAttestationMode(value string) (AttestationMode, error) {
	switch value {
	case "", string(AttestationOff):
		return AttestationOff, nil
	case string(AttestationAuto):
		return AttestationAuto, nil
	case string(AttestationRequired):
		return AttestationRequired, nil
	default:
		return "", NewAttestationModeError(value)
	}
}

type AttestationModeError struct {
	Value string
}

func NewAttestationModeError(value string) *AttestationModeError {
	return &AttestationModeError{Value: value}
}

func (e *AttestationModeError) Error() string {
	return "invalid attestation mode " + e.Value + " (expected off, auto, or required)"
}

type AttestationResult string

const (
	AttestationResultUnknown                 AttestationResult = ""
	AttestationResultVerified                AttestationResult = "verified"
	AttestationResultSkippedOff              AttestationResult = "skipped-off"
	AttestationResultSkippedGitHubCLIMissing AttestationResult = "skipped-github-cli-missing"
)

func (r AttestationResult) Display() string {
	switch r {
	case AttestationResultVerified:
		return "Verified"
	case AttestationResultSkippedOff:
		return "Skipped (off)"
	case AttestationResultSkippedGitHubCLIMissing:
		return "Skipped (GitHub CLI not found on PATH)"
	default:
		return ""
	}
}
