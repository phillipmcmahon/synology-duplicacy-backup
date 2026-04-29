package command

import (
	"fmt"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	updatepkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/update"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func UpdateUsageText(meta workflow.Metadata, rt workflow.Env) string {
	return scriptTemplate(meta, `Usage: {{script}} update [OPTIONS]

Update options:
    --check-only
    --force
    --yes
    --keep <count>                       (default: {{default_keep}})
    --version <tag>                      (default: latest)
    --attestations <off|auto|required>   (default: off)
    --config-dir <path>                  (default: $HOME/.config/duplicacy-backup)
    --help
    --help-full

Examples:
    {{script}} update --check-only
    {{script}} update --yes
    {{script}} update --attestations required --yes
    {{script}} update --version v4.1.8 --yes
    {{script}} update --yes --config-dir "$HOME/.config/duplicacy-backup"

Use --help-full for the detailed update reference.
`,
		"{{default_keep}}", fmt.Sprint(updatepkg.DefaultKeep),
	)
}

func RollbackUsageText(meta workflow.Metadata, rt workflow.Env) string {
	return scriptTemplate(meta, `Usage: {{script}} rollback [OPTIONS]

Rollback options:
    --check-only
    --yes
    --version <tag>       (default: newest previous retained version)
    --help
    --help-full

Examples:
    {{script}} rollback --check-only
    sudo {{script}} rollback --yes
    sudo {{script}} rollback --version v5.1.1 --yes

Use --help-full for the detailed rollback reference.
`)
}

func FullRollbackUsageText(meta workflow.Metadata, rt workflow.Env) string {
	return scriptTemplate(meta, `Usage: {{script}} rollback [OPTIONS]

ROLLBACK BEHAVIOUR:
    rollback:
      - inspects the managed install layout used by update
      - lists retained versioned binaries in the install root
      - activates the newest previous retained version by default
      - can activate an explicit retained version with --version
      - updates only the managed current symlink
      - does not download releases and does not modify config or secrets

OPTIONS:
    --check-only           Show the rollback plan without changing symlinks
    --yes                  Skip the interactive confirmation prompt
    --version <tag>        Activate one retained version instead of the newest previous version
    --help                 Show the concise rollback help message
    --help-full            Show the detailed rollback help message

INTERACTIVE RULES:
    Interactive runs show the selected rollback target and ask for confirmation.
    Non-interactive activation requires --yes.
    --check-only is safe to run without root because it does not change symlinks.

SUPPORTED LAYOUT:
    rollback expects the standard managed install layout:
      /usr/local/lib/duplicacy-backup/
      /usr/local/bin/duplicacy-backup -> /usr/local/lib/duplicacy-backup/current

EXAMPLES:
    {{script}} rollback --check-only
    sudo {{script}} rollback --yes
    sudo {{script}} rollback --version v5.1.1 --yes
`)
}

func FullUpdateUsageText(meta workflow.Metadata, rt workflow.Env) string {
	cfgDir := workflow.EffectiveConfigDir(rt)
	return scriptTemplate(meta, `Usage: {{script}} update [OPTIONS]

UPDATE BEHAVIOUR:
    update:
      - checks GitHub releases for the latest published version by default
      - downloads the matching Linux package for the current platform
      - verifies the package checksum before extracting it
      - reuses the packaged install.sh to switch the installation safely
      - only works on the supported managed install layout

OPTIONS:
    --check-only           Show the planned update without downloading or installing
    --force                Reinstall even when the selected release is already current
    --yes                  Skip the interactive confirmation prompt
    --keep <count>         Keep this many newest installed binaries after activation (default: {{default_keep}})
    --version <tag>        Install one specific published release tag instead of the latest release
    --attestations <mode>  Verify GitHub release attestations: off, auto, or required (default: off)
    --config-dir <path>    Override config directory for update notifications (default: $HOME/.config/duplicacy-backup)
    --help                 Show the concise update help message
    --help-full            Show the detailed update help message

INTERACTIVE RULES:
    Interactive runs show the detected install plan and ask for confirmation before install.
    Non-interactive runs require --yes for the install step.
    --force changes version selection behaviour only; it does not skip confirmation.

ATTESTATION VERIFICATION:
    off:
      - default mode; keeps existing NAS update jobs unchanged
      - verifies only the published SHA256 checksum before extraction
    auto:
      - verifies the downloaded tarball with gh release verify-asset when
        GitHub CLI is available on PATH
      - skips attestation verification if gh is not installed
      - fails before extraction/install if gh is available but verification fails
    required:
      - requires GitHub CLI on PATH
      - fails before extraction/install if attestation verification is unavailable
        or unsuccessful

SUPPORTED LAYOUT:
    update expects the standard managed install layout:
      /usr/local/lib/duplicacy-backup/
      /usr/local/bin/duplicacy-backup -> /usr/local/lib/duplicacy-backup/current
    If the running binary is outside that layout, update stops and asks for a manual install.

UPDATE NOTIFICATIONS:
    update notification config is global, not label/target scoped:
      {{config_dir}}/{{app_config_file}}

    Example:
      [update.notify]
      notify_on = ["failed"]
      interactive = false

      [update.notify.ntfy]
      url = "https://ntfy.sh"
      topic = "duplicacy-updates"

    Failure notifications do not read label storage secrets. If no update
    notification config is present, update still runs normally and Synology
    scheduled-task monitoring remains the fallback for hard failures.

EXAMPLES:
    {{script}} update --check-only
    {{script}} update --yes
    {{script}} update --attestations required --yes
    {{script}} update --force --yes
    {{script}} update --keep 3 --yes
    {{script}} update --version v4.1.8 --yes
`,
		"{{default_keep}}", fmt.Sprint(updatepkg.DefaultKeep),
		"{{config_dir}}", cfgDir,
		"{{app_config_file}}", config.DefaultAppConfigFile,
	)
}
