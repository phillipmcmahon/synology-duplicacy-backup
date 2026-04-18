package update

import (
	"fmt"
	"os/exec"
	"strings"
)

func (u *Updater) verifyReleaseAssetAttestation(planned *plan, assetPath string) error {
	switch planned.Attestations {
	case AttestationOff:
		planned.Attestation = AttestationResultSkippedOff
		return nil
	case AttestationAuto, AttestationRequired:
	default:
		return NewAttestationModeError(string(planned.Attestations))
	}

	if _, err := u.Runtime.LookPath("gh"); err != nil {
		if planned.Attestations == AttestationRequired {
			return fmt.Errorf("release attestation verification requires GitHub CLI (gh) on PATH; install gh or use --attestations off: %w", err)
		}
		planned.Attestation = AttestationResultSkippedGitHubCLIMissing
		return nil
	}

	output, err := u.VerifyAsset(planned.ReleaseTag, u.Repo, assetPath)
	if err != nil {
		return fmt.Errorf("release attestation verification failed for %s: %w%s", planned.AssetName, err, formatCommandOutput(output))
	}
	planned.Attestation = AttestationResultVerified
	return nil
}

func runGHReleaseVerifyAsset(tag, repo, assetPath string) ([]byte, error) {
	cmd := exec.Command("gh", "release", "verify-asset", tag, assetPath, "--repo", repo)
	return cmd.CombinedOutput()
}

func formatCommandOutput(output []byte) string {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return ""
	}
	return "\n" + text
}
