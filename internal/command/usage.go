package command

import (
	"fmt"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	updatepkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/update"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func UsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	return fmt.Sprintf(`Usage: %s [OPTIONS] <source>
       %s config <validate|explain|paths> [OPTIONS] <source>
       %s notify <test> [OPTIONS] <source|update>
       %s update [OPTIONS]
       %s health <status|doctor|verify> [OPTIONS] <source>

Operations:
    --backup
    --prune
    --cleanup-storage
    --fix-perms

Execution order:
    backup -> prune -> cleanup-storage -> fix-perms

Common modifiers:
    --force-prune
    --target <name>
    --dry-run
    --verbose
    --json-summary
    --config-dir <path>
    --secrets-dir <path>
    --version, -v
    --help
    --help-full

Examples:
    %s --target onsite-usb --backup homes
    %s --target onsite-usb --backup --prune homes
    %s --target onsite-usb --json-summary --dry-run --backup homes
    %s --target offsite-storj --backup homes
    %s config validate --target onsite-usb homes
    %s notify test --target onsite-usb homes
    %s update --check-only
    %s health status --target onsite-usb homes

Use --help-full for the detailed reference.
`,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
	)
}

func FullUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	cfgDir := workflow.EffectiveConfigDir(rt)
	return fmt.Sprintf(`Usage: %s [OPTIONS] <source>
       %s config <validate|explain|paths> [OPTIONS] <source>
       %s notify <test> [OPTIONS] <source|update>
       %s update [OPTIONS]
       %s health <status|doctor|verify> [OPTIONS] <source>

OPERATIONS:
    Operations may be combined. Execution order is fixed:
      1. backup
      2. prune
      3. cleanup-storage
      4. fix-perms

    --backup                 Request backup
    --prune                  Request threshold-guarded prune
    --cleanup-storage        Request storage maintenance:
                             duplicacy prune -exhaustive -exclusive
                             Use only when no other client is writing to the same storage
    --fix-perms              Normalise filesystem repository ownership and permissions
    At least one operation flag is required for runtime commands

MODIFIERS:
    --force-prune            Override safe prune thresholds during prune
    --target <name>          Select the named target config where the command uses a label target
    --dry-run                Simulate actions without making changes
    --verbose                Show detailed operational logging and command details
    --json-summary           Write a machine-readable run summary to stdout
    --config-dir <path>      Override config directory (default: <binary-dir>/.config)
    --secrets-dir <path>     Override secrets directory (default: %s)
    --version, -v            Show version and build information
    --help                   Show the concise help message
    --help-full              Show the detailed help message

HEALTH COMMANDS:
    health status            Fast read-only health summary for operators and schedulers
    health doctor            Read-only environment and storage diagnostics
    health verify            Read-only integrity check across revisions found for the current label

NOTIFY COMMANDS:
    notify test             Send a clearly marked simulated notification through the configured providers
    notify test update      Send a simulated update notification through the global update config

UPDATE COMMAND:
    update                  Check GitHub for a newer published release and install it through the packaged installer

ENVIRONMENT VARIABLES:
    DUPLICACY_BACKUP_CONFIG_DIR   Override config directory (--config-dir takes precedence)
    DUPLICACY_BACKUP_SECRETS_DIR  Override secrets directory (--secrets-dir takes precedence)

SAFE PRUNE THRESHOLDS:
    Max delete percent       : %d%% (default %d%%)
    Max delete count         : %d (default %d)
    Min revisions for %% check: %d (default %d)

CONFIG FILE LOCATION:
    <binary-dir>/.config/<label>-backup.toml
    Effective default: %s/<label>-backup.toml
    Override with --config-dir or DUPLICACY_BACKUP_CONFIG_DIR
    Global app config, when used: %s/%s

CONFIG STRUCTURE:
    label config files define:
      source_path
      [common]
      [health]
      [health.notify]
      [targets.<name>]
      optional [targets.<name>.health]
      optional [targets.<name>.health.notify]
    each [targets.<name>] entry must include:
      type = "filesystem" | "duplicacy"
      location = "local" | "remote"
    supported type/location pairs:
      filesystem/local, filesystem/remote, duplicacy/local, duplicacy/remote

    TARGET SECRETS:
      Duplicacy storage targets load runtime preference keys from:
        %s/<label>-secrets.toml
      Filesystem targets do not use storage credentials, even when location = "remote"
      Any target may also store optional health_webhook_bearer_token / health_ntfy_token there
      Override directory with --secrets-dir or DUPLICACY_BACKUP_SECRETS_DIR
      Use [targets.<name>.keys] tables with Duplicacy key names such as:
        s3_id
        s3_secret
      Use [targets.<name>] for:
        optional health_webhook_bearer_token
        optional health_ntfy_token

    CONFIG VALIDATE PERMISSIONS:
      config validate reports:
        Privileges : Full     when root-only checks can run
        Privileges : Limited  when root-only checks may be shown as Not checked

HEALTH STATE:
    Target-specific run and health state are stored under:
      %s/<label>.<target>.json
    Health commands combine this state with live storage inspection.

HEALTH CONFIG:
    Optional [health] table keys:
      freshness_warn_hours
      freshness_fail_hours
      doctor_warn_after_hours
      verify_warn_after_hours

    Optional [health.notify] table keys:
      webhook_url
      optional [health.notify.ntfy]:
        url = "https://ntfy.sh"
        topic = "duplicacy-alerts"
      notify_on = ["degraded", "unhealthy"]
      send_for = ["doctor", "verify"]  # add backup, prune, cleanup-storage to opt runtime alerts in
      interactive = false

    Optional secrets key:
      health_webhook_bearer_token
      health_ntfy_token

UPDATE NOTIFY CONFIG:
    Optional global update notification config lives in %s/%s:
      [update.notify]
      notify_on = ["failed"]
      interactive = false

      [update.notify.ntfy]
      url = "https://ntfy.sh"
      topic = "duplicacy-updates"

    Update notifications are not label/target scoped and do not read storage secrets.

ARGUMENTS:
    source                   Backup label

INTERACTIVE SAFETY RAILS:
    Interactive terminal runs ask for confirmation before:
      - forced prune
      - cleanup-storage
    Non-interactive runs continue without confirmation so scheduled jobs are unaffected

JSON SUMMARY:
    --json-summary writes a machine-readable completion summary to stdout.
    Human-readable logs continue to be written to stderr.

EXAMPLES:
    %s --target onsite-usb --backup homes
    %s --target onsite-usb --backup homes
    %s --target onsite-usb --backup --prune homes
    %s --target onsite-usb --json-summary --dry-run --backup homes
    %s health status --target onsite-usb homes
    %s health doctor --json-summary --target onsite-usb homes
    %s health verify --target offsite-storj homes
    %s --target onsite-usb --prune homes
    %s --target onsite-usb --cleanup-storage homes
    %s --target onsite-usb --prune --cleanup-storage homes
    %s --target onsite-usb --prune --force-prune --cleanup-storage homes
    %s --target onsite-usb --backup --prune --force-prune --cleanup-storage homes
    %s --target onsite-usb --fix-perms homes
    %s --target onsite-usb --backup --fix-perms homes
    %s --target offsite-storj --backup homes
    %s --target onsite-usb --verbose --backup --prune homes
    %s --target onsite-usb --config-dir /opt/etc --backup homes
    %s --secrets-dir /opt/secrets --target offsite-storj --backup homes
    %s config validate --target onsite-usb homes
    %s config explain --target offsite-storj homes
    %s config paths --target onsite-usb homes
    %s notify test --target onsite-usb homes
    %s update --check-only
    %s update --yes
`,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		config.DefaultSecretsDir,
		config.DefaultSafePruneMaxDeletePercent, config.DefaultSafePruneMaxDeletePercent,
		config.DefaultSafePruneMaxDeleteCount, config.DefaultSafePruneMaxDeleteCount,
		config.DefaultSafePruneMinTotalForPercent, config.DefaultSafePruneMinTotalForPercent,
		cfgDir,
		cfgDir,
		config.DefaultAppConfigFile,
		config.DefaultSecretsDir,
		meta.StateDir,
		cfgDir,
		config.DefaultAppConfigFile,
		meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName,
		meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName,
		meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName,
		meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName,
		meta.ScriptName, meta.ScriptName, meta.ScriptName, meta.ScriptName,
	)
}

func ConfigUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	return fmt.Sprintf(`Usage: %s config <validate|explain|paths> [OPTIONS] <source>

Config commands:
    validate
    explain
    paths

Options:
    --target <name>
    --config-dir <path>     (default: <binary-dir>/.config)
    --secrets-dir <path>    (default: %s)
    --help
    --help-full

Examples:
    %s config validate --target onsite-usb homes
    %s config explain --target offsite-storj homes
    %s config paths --target onsite-usb homes

Use --help-full for the detailed config reference.
`,
		meta.ScriptName,
		config.DefaultSecretsDir,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
	)
}

func NotifyUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	return fmt.Sprintf(`Usage: %s notify <test> [OPTIONS] <source|update>

Notify commands:
    test

Options:
    --target <name>
    --provider <all|webhook|ntfy>        (default: all)
    --severity <warning|critical|info>   (default: warning)
    --event <name>
    --summary <text>
    --message <text>
    --dry-run
    --json-summary
    --config-dir <path>                  (default: <binary-dir>/.config)
    --secrets-dir <path>                 (default: %s)
    --help
    --help-full

Examples:
    %s notify test --target onsite-usb homes
    %s notify test --target offsite-storj --provider ntfy homes
    %s notify test update --provider ntfy --dry-run
    %s notify test --target onsite-usb --dry-run homes

Use --help-full for the detailed notify reference.
`,
		meta.ScriptName,
		config.DefaultSecretsDir,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
	)
}

func FullNotifyUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	return fmt.Sprintf(`Usage: %s notify <test> [OPTIONS] <source|update>

NOTIFY COMMANDS:
    test                    Send a synthetic test notification through the configured providers for the selected label and target
    test update             Send a synthetic update notification through the global update notification config

OPTIONS:
    --target <name>         Select the configured target to test (required for label/target tests)
    --provider <name>       One of all, webhook, or ntfy (default: all)
    --severity <level>      One of warning, critical, or info (default: warning)
    --event <name>          Update event to simulate for notify test update
    --summary <text>        Override the default test summary line
    --message <text>        Override the default synthetic message body
    --dry-run               Preview the resolved destinations and synthetic payload without sending
    --json-summary          Write a machine-readable test summary to stdout
    --config-dir <path>     Override config directory (default: <binary-dir>/.config)
    --secrets-dir <path>    Override secrets directory (default: %s)
    --help                  Show the concise notify help message
    --help-full             Show the detailed notify help message

BEHAVIOUR:
    notify test:
      - uses the existing label and target config
      - uses target-scoped notification auth tokens when configured
      - sends a clearly marked synthetic notification
      - bypasses notify_on / send_for gating because it is an explicit operator test
      - fails if the selected provider is not configured for the selected target

    notify test update:
      - uses the global app config at <config-dir>/%s
      - does not require a label, target, or storage secrets
      - sends a synthetic update notification; default event is update_install_failed
      - bypasses update.notify.notify_on because it is an explicit operator test

PROVIDER SELECTION:
    --provider all          Test every configured destination for the selected target
    --provider webhook      Test only the configured webhook destination
    --provider ntfy         Test only the configured ntfy destination

EXAMPLES:
    %s notify test --target onsite-usb homes
    %s notify test --target offsite-storj --provider ntfy homes
    %s notify test --target onsite-usb --severity critical homes
    %s notify test --target onsite-usb --summary "NAS alert path test" --message "Synthetic end-to-end notification check" homes
    %s notify test --target onsite-usb --dry-run homes
    %s notify test --target onsite-usb --json-summary homes
    %s notify test update --provider ntfy --event update_install_failed
    %s notify test update --provider ntfy --severity critical --dry-run
`,
		meta.ScriptName,
		config.DefaultSecretsDir,
		config.DefaultAppConfigFile,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
	)
}

func UpdateUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	return fmt.Sprintf(`Usage: %s update [OPTIONS]

Update options:
    --check-only
    --force
    --yes
    --keep <count>                       (default: %d)
    --version <tag>                      (default: latest)
    --attestations <off|auto|required>   (default: off)
    --config-dir <path>                  (default: <binary-dir>/.config)
    --help
    --help-full

Examples:
    %s update --check-only
    %s update --yes
    %s update --attestations required --yes
    %s update --version v4.1.8 --yes
    %s update --yes --config-dir /usr/local/lib/duplicacy-backup/.config

Use --help-full for the detailed update reference.
`,
		meta.ScriptName,
		updatepkg.DefaultKeep,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
	)
}

func FullUpdateUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	cfgDir := workflow.EffectiveConfigDir(rt)
	return fmt.Sprintf(`Usage: %s update [OPTIONS]

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
    --keep <count>         Keep this many newest installed binaries after activation (default: %d)
    --version <tag>        Install one specific published release tag instead of the latest release
    --attestations <mode>  Verify GitHub release attestations: off, auto, or required (default: off)
    --config-dir <path>    Override config directory for update notifications (default: <binary-dir>/.config)
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
      %s/%s

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
    %s update --check-only
    %s update --yes
    %s update --attestations required --yes
    %s update --force --yes
    %s update --keep 3 --yes
    %s update --version v4.1.8 --yes
`,
		meta.ScriptName,
		updatepkg.DefaultKeep,
		cfgDir,
		config.DefaultAppConfigFile,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
	)
}

func FullConfigUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	cfgDir := workflow.EffectiveConfigDir(rt)
	return fmt.Sprintf(`Usage: %s config <validate|explain|paths> [OPTIONS] <source>

CONFIG COMMANDS:
    validate                Validate the resolved config and configured secrets
    explain                 Show the resolved config values for the selected target
    paths                   Show the resolved stable config, source, log, and any applicable secrets paths

OPTIONS:
    --target <name>         Select the named target (required)
    --config-dir <path>     Override config directory (default: <binary-dir>/.config)
    --secrets-dir <path>    Override secrets directory (default: %s)
    --help                  Show the concise help message
    --help-full             Show the detailed config help message

BEHAVIOUR:
    validate, explain, and paths operate on one selected target from a label config at a time.
    Every config command requires an explicit --target selection.

DEFAULT LOCATIONS:
    Config dir             : %s
    Secrets dir            : %s

EXAMPLES:
    %s config validate --target onsite-usb homes
    %s config validate --target offsite-storj homes
    %s config explain --target onsite-usb homes
    %s config explain --target offsite-storj homes
    %s config paths --target onsite-usb homes
`,
		meta.ScriptName,
		config.DefaultSecretsDir,
		cfgDir,
		config.DefaultSecretsDir,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
		meta.ScriptName,
	)
}

func VersionText(meta workflow.Metadata) string {
	return fmt.Sprintf("%s %s (built %s)\n", meta.ScriptName, meta.Version, meta.BuildTime)
}
