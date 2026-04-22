package command

import (
	"fmt"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	updatepkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/update"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func UsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	return scriptTemplate(meta, `Usage: {{script}} <command> [OPTIONS] <source>
       {{script}} backup [OPTIONS] <source>
       {{script}} prune [OPTIONS] <source>
       {{script}} cleanup-storage [OPTIONS] <source>
       {{script}} fix-perms [OPTIONS] <source>
       {{script}} config <validate|explain|paths> [OPTIONS] <source>
       {{script}} diagnostics [OPTIONS] <source>
       {{script}} health <status|doctor|verify> [OPTIONS] <source>
       {{script}} notify <test> [OPTIONS] <source|update>
       {{script}} restore <plan|prepare> [OPTIONS] <source>
       {{script}} update [OPTIONS]
       {{script}} rollback [OPTIONS]

Commands:
    Runtime operations     backup, prune, cleanup-storage, fix-perms
    Config and inspection  config, diagnostics, health
    Notifications          notify test, notify test update
    Restore drills         restore plan, restore prepare
    Managed install        update, rollback

Common options:
    --target <name>        Select a configured label target where required
    --dry-run              Preview supported operations without making changes
    --verbose              Show detailed operational output where supported
    --json-summary         Write supported command summaries as JSON
    --config-dir <path>    Override config directory
    --secrets-dir <path>   Override secrets directory
    --help                 Show concise help
    --help-full            Show detailed help
    --version, -v          Show version

Command-specific options:
    --force                Prune: override thresholds; update: reinstall selected release
    --workspace <path>     Override restore drill workspace
    --provider <name>      Select notification provider for notify test
    --check-only           Inspect update or rollback without changing install
    --yes                  Skip update or rollback confirmation
    --keep <count>         Update retention count
    --attestations <mode>  Update release attestation mode

Examples:
    {{script}} backup --target onsite-usb homes
    {{script}} backup --target onsite-usb --json-summary --dry-run homes
    {{script}} backup --target offsite-storj homes
    {{script}} prune --target onsite-usb homes
    {{script}} config validate --target onsite-usb homes
    {{script}} diagnostics --target onsite-usb homes
    {{script}} health status --target onsite-usb homes
    {{script}} notify test --target onsite-usb homes
    {{script}} restore plan --target onsite-usb homes
    {{script}} restore prepare --target onsite-usb homes
    {{script}} update --check-only
    {{script}} rollback --check-only

Use --help-full for the detailed reference.
`)
}

func scriptTemplate(meta workflow.Metadata, template string, replacements ...string) string {
	pairs := []string{"{{script}}", meta.ScriptName}
	pairs = append(pairs, replacements...)
	return strings.NewReplacer(pairs...).Replace(template)
}

func FullUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	cfgDir := workflow.EffectiveConfigDir(rt)
	return scriptTemplate(meta, `Usage: {{script}} <command> [OPTIONS] <source>
       {{script}} backup [OPTIONS] <source>
       {{script}} prune [OPTIONS] <source>
       {{script}} cleanup-storage [OPTIONS] <source>
       {{script}} fix-perms [OPTIONS] <source>
       {{script}} config <validate|explain|paths> [OPTIONS] <source>
       {{script}} diagnostics [OPTIONS] <source>
       {{script}} health <status|doctor|verify> [OPTIONS] <source>
       {{script}} notify <test> [OPTIONS] <source|update>
       {{script}} restore <plan|prepare> [OPTIONS] <source>
       {{script}} update [OPTIONS]
       {{script}} rollback [OPTIONS]

COMMAND OVERVIEW:
    Runtime operations      Run, maintain, or repair one configured label target
      backup                Run a backup for the selected label and target
      prune                 Run threshold-guarded prune for the selected label and target
      cleanup-storage       Request storage maintenance:
                              duplicacy prune -exhaustive -exclusive
                              Use only when no other client is writing to the same storage
      fix-perms             Normalise path-based storage ownership and permissions

    Config and inspection   Read, explain, validate, or diagnose configured targets
      config validate       Validate the resolved config and configured secrets
      config explain        Show resolved config values for the selected target
      config paths          Show resolved config, source, log, and secrets paths
      diagnostics           Print a redacted support bundle for one label and target
      health status         Fast read-only health summary for operators and schedulers
      health doctor         Read-only environment and storage diagnostics
      health verify         Read-only integrity check across revisions found for the current label

    Notifications           Send explicit synthetic notification checks
      notify test           Send a simulated notification through configured providers
      notify test update    Send a simulated update notification through global update config

    Restore drills          Prepare safe restore workflows without writing to the live source
      restore plan          Print a read-only Duplicacy restore-drill plan without executing a restore
      restore prepare       Prepare a safe drill workspace without executing a restore

    Managed install         Manage the installed application binary
      update                Check GitHub for a newer published release and install it through the packaged installer
      rollback              Inspect or activate a retained managed-install version

COMMON OPTIONS:
    --target <name>          Select the named target config where the command uses a label target
    --dry-run                Simulate supported actions without making changes
    --verbose                Show detailed operational logging and command details
    --json-summary           Write a machine-readable command summary to stdout
    --config-dir <path>      Override config directory (default: <binary-dir>/.config)
    --secrets-dir <path>     Override secrets directory (default: {{default_secrets_dir}})
    --help                   Show the concise help message
    --help-full              Show the detailed help message
    --version, -v            Show version and build information

COMMAND-SPECIFIC OPTIONS:
    --force                  Prune: override thresholds; update: reinstall selected release
    --workspace <path>       Override restore drill workspace
    --provider <name>        Select notification provider for notify test
    --severity <level>       Select notification severity for notify test
    --event <name>           Select notification event for notify test
    --check-only             Inspect update or rollback without changing install
    --yes                    Skip update or rollback confirmation
    --keep <count>           Update retention count (default: {{default_keep}})
    --attestations <mode>    Update release attestation mode

ENVIRONMENT VARIABLES:
    DUPLICACY_BACKUP_CONFIG_DIR   Override config directory (--config-dir takes precedence)
    DUPLICACY_BACKUP_SECRETS_DIR  Override secrets directory (--secrets-dir takes precedence)

SAFE PRUNE THRESHOLDS:
    Max delete percent       : {{safe_prune_max_delete_percent}}% (default {{safe_prune_max_delete_percent}}%)
    Max delete count         : {{safe_prune_max_delete_count}} (default {{safe_prune_max_delete_count}})
    Min revisions for % check: {{safe_prune_min_total_for_percent}} (default {{safe_prune_min_total_for_percent}})

CONFIG FILE LOCATION:
    <binary-dir>/.config/<label>-backup.toml
    Effective default: {{config_dir}}/<label>-backup.toml
    Override with --config-dir or DUPLICACY_BACKUP_CONFIG_DIR
    Global app config, when used: {{config_dir}}/{{app_config_file}}

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
      location = "local" | "remote"
      storage = "<duplicacy storage value>"

    TARGET SECRETS:
      Duplicacy storage targets load runtime preference keys from:
        {{default_secrets_dir}}/<label>-secrets.toml
      Path-based Duplicacy storage targets do not use storage credentials
      Any target may also store optional health_webhook_bearer_token / health_ntfy_token there
      Override directory with --secrets-dir or DUPLICACY_BACKUP_SECRETS_DIR
      Use [targets.<name>.keys] tables with Duplicacy key names such as:
        s3_id      # s3, s3c, minio, minios
        s3_secret  # s3, s3c, minio, minios
      Use [targets.<name>] for:
        optional health_webhook_bearer_token
        optional health_ntfy_token

    CONFIG VALIDATE PERMISSIONS:
      config validate reports:
        Privileges : Full     when root-only checks can run
        Privileges : Limited  when root-only checks may be shown as Not checked

HEALTH STATE:
    Target-specific run and health state are stored under:
      {{state_dir}}/<label>.<target>.json
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
    Optional global update notification config lives in {{config_dir}}/{{app_config_file}}:
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

COMMAND MODEL:
    Runtime operations are first-class commands. Use "backup --target ...",
    "prune --target ...", "cleanup-storage --target ...", or
    "fix-perms --target ..."; old top-level operation flags are not supported.

JSON SUMMARY:
    --json-summary writes a machine-readable completion summary to stdout.
    Human-readable logs continue to be written to stderr.

RESTORE PLANNING:
    restore plan is read-only. It resolves label and target context, shows the
    safe drill workspace pattern, and prints Duplicacy commands to run manually.
    It does not create directories, write preferences, run duplicacy restore, or
    copy data back to the live source path.

RESTORE PREPARATION:
    restore prepare creates a separate drill workspace and writes the Duplicacy
    preferences needed to inspect or restore manually from that workspace. It
    rejects the live source path, source-child workspaces, and non-empty
    workspaces. It does not run duplicacy restore or copy data back.

EXAMPLES:
    {{script}} backup --target onsite-usb homes
    {{script}} backup --target onsite-usb --json-summary --dry-run homes
    {{script}} health status --target onsite-usb homes
    {{script}} health doctor --json-summary --target onsite-usb homes
    {{script}} health verify --target offsite-storj homes
    {{script}} prune --target onsite-usb homes
    {{script}} prune --target onsite-usb --force homes
    {{script}} cleanup-storage --target onsite-usb homes
    {{script}} fix-perms --target onsite-usb homes
    {{script}} backup --target offsite-storj homes
    {{script}} backup --target onsite-usb --verbose homes
    {{script}} backup --target onsite-usb --config-dir /opt/etc homes
    {{script}} backup --secrets-dir /opt/secrets --target offsite-storj homes
    {{script}} config validate --target onsite-usb homes
    {{script}} config explain --target offsite-storj homes
    {{script}} config paths --target onsite-usb homes
    {{script}} diagnostics --target onsite-usb homes
    {{script}} restore plan --target onsite-usb homes
    {{script}} restore plan --target offsite-storj homes
    {{script}} restore prepare --target onsite-usb homes
    {{script}} restore prepare --target offsite-storj --workspace /volume1/restore-drills/homes-offsite-storj homes
    {{script}} notify test --target onsite-usb homes
    {{script}} rollback --check-only
    {{script}} rollback --yes
    {{script}} update --check-only
    {{script}} update --yes
`,
		"{{default_secrets_dir}}", config.DefaultSecretsDir,
		"{{default_keep}}", fmt.Sprint(updatepkg.DefaultKeep),
		"{{safe_prune_max_delete_percent}}", fmt.Sprint(config.DefaultSafePruneMaxDeletePercent),
		"{{safe_prune_max_delete_count}}", fmt.Sprint(config.DefaultSafePruneMaxDeleteCount),
		"{{safe_prune_min_total_for_percent}}", fmt.Sprint(config.DefaultSafePruneMinTotalForPercent),
		"{{config_dir}}", cfgDir,
		"{{app_config_file}}", config.DefaultAppConfigFile,
		"{{state_dir}}", meta.StateDir,
	)
}

func ConfigUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	return scriptTemplate(meta, `Usage: {{script}} config <validate|explain|paths> [OPTIONS] <source>

Config commands:
    validate
    explain
    paths

Options:
    --target <name>
    --config-dir <path>     (default: <binary-dir>/.config)
    --secrets-dir <path>    (default: {{default_secrets_dir}})
    --help
    --help-full

Examples:
    {{script}} config validate --target onsite-usb homes
    {{script}} config explain --target offsite-storj homes
    {{script}} config paths --target onsite-usb homes

Use --help-full for the detailed config reference.
`,
		"{{default_secrets_dir}}", config.DefaultSecretsDir,
	)
}

func DiagnosticsUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	return scriptTemplate(meta, `Usage: {{script}} diagnostics [OPTIONS] <source>

Diagnostics options:
    --target <name>
    --json-summary
    --config-dir <path>     (default: <binary-dir>/.config)
    --secrets-dir <path>    (default: {{default_secrets_dir}})
    --help
    --help-full

Examples:
    {{script}} diagnostics --target onsite-usb homes
    {{script}} diagnostics --target offsite-storj --json-summary homes

Use --help-full for the detailed diagnostics reference.
`,
		"{{default_secrets_dir}}", config.DefaultSecretsDir,
	)
}

func FullDiagnosticsUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	cfgDir := workflow.EffectiveConfigDir(rt)
	return scriptTemplate(meta, `Usage: {{script}} diagnostics [OPTIONS] <source>

DIAGNOSTICS COMMAND:
    diagnostics             Print a redacted support bundle for one label and target

OPTIONS:
    --target <name>         Select the named target (required)
    --json-summary          Write the diagnostics report as machine-readable JSON
    --config-dir <path>     Override config directory (default: <binary-dir>/.config)
    --secrets-dir <path>    Override secrets directory (default: {{default_secrets_dir}})
    --help                  Show the concise diagnostics help message
    --help-full             Show the detailed diagnostics help message

BEHAVIOUR:
    diagnostics:
      - resolves one label and target without running backup, prune, restore, or storage maintenance
      - reports config paths, source path, storage value, storage scheme, state freshness, and last known run details
      - inspects local path accessibility when the selected storage is path-based
      - redacts secrets and reports only whether required storage keys are present
      - can be pasted into support conversations without exposing storage credentials or notification tokens

DEFAULT LOCATIONS:
    Config dir             : {{config_dir}}
    Secrets dir            : {{default_secrets_dir}}
    State dir              : {{state_dir}}
    Log dir                : {{log_dir}}

EXAMPLES:
    {{script}} diagnostics --target onsite-usb homes
    {{script}} diagnostics --target offsite-storj homes
    {{script}} diagnostics --target offsite-storj --json-summary homes
    {{script}} diagnostics --target onsite-usb --config-dir /opt/etc --secrets-dir /opt/secrets homes
`,
		"{{default_secrets_dir}}", config.DefaultSecretsDir,
		"{{config_dir}}", cfgDir,
		"{{state_dir}}", meta.StateDir,
		"{{log_dir}}", meta.LogDir,
	)
}

func RestoreUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	return scriptTemplate(meta, `Usage: {{script}} restore <plan|prepare> [OPTIONS] <source>

Restore commands:
    plan
    prepare

Options:
    --target <name>
    --workspace <path>      (default: <source-volume>/restore-drills/<label>-<target>)
    --config-dir <path>     (default: <binary-dir>/.config)
    --secrets-dir <path>    (default: {{default_secrets_dir}})
    --help
    --help-full

Examples:
    {{script}} restore plan --target onsite-usb homes
    {{script}} restore prepare --target onsite-usb homes
    {{script}} restore prepare --target offsite-storj --workspace /volume1/restore-drills/homes-offsite-storj homes
    {{script}} restore plan --target offsite-storj homes

Use --help-full for the detailed restore reference.
`,
		"{{default_secrets_dir}}", config.DefaultSecretsDir,
	)
}

func FullRestoreUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	cfgDir := workflow.EffectiveConfigDir(rt)
	return scriptTemplate(meta, `Usage: {{script}} restore <plan|prepare> [OPTIONS] <source>

RESTORE COMMANDS:
    plan                   Resolve a safe read-only restore drill plan for one label and target
    prepare                Create a safe drill workspace and write Duplicacy preferences

OPTIONS:
    --target <name>        Select the named target (required)
    --workspace <path>     Override drill workspace (default: <source-volume>/restore-drills/<label>-<target>)
    --config-dir <path>    Override config directory (default: <binary-dir>/.config)
    --secrets-dir <path>   Override secrets directory (default: {{default_secrets_dir}})
    --help                 Show the concise restore help message
    --help-full            Show the detailed restore help message

BEHAVIOUR:
    restore plan:
      - reads the selected label config
      - shows the resolved source path, storage value, config file, and applicable secrets file
      - reads the target state file when available to show the latest known backup revision
      - prints Duplicacy commands for creating a separate drill workspace, listing revisions, and restoring manually
      - does not create directories, write Duplicacy preferences, execute duplicacy restore, or copy data back
    restore prepare:
      - creates the selected drill workspace when it does not exist
      - writes .duplicacy/preferences for the selected storage target
      - rejects the live source path, source-child workspaces, and non-empty workspaces
      - does not run duplicacy restore or copy data back

DEFAULT LOCATIONS:
    Config dir             : {{config_dir}}
    Secrets dir            : {{default_secrets_dir}}

SAFETY MODEL:
    Restore drills should restore into a separate workspace first. Inspect the
    restored data there, then copy back deliberately with rsync --dry-run before
    any live write. See docs/restore-drills.md for the full procedure.

EXAMPLES:
    {{script}} restore plan --target onsite-usb homes
    {{script}} restore plan --target offsite-storj homes
    {{script}} restore prepare --target onsite-usb homes
    {{script}} restore prepare --target offsite-storj --workspace /volume1/restore-drills/homes-offsite-storj homes
    {{script}} restore plan --target offsite-storj --config-dir /opt/etc --secrets-dir /opt/secrets homes
`,
		"{{default_secrets_dir}}", config.DefaultSecretsDir,
		"{{config_dir}}", cfgDir,
	)
}

func NotifyUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	return scriptTemplate(meta, `Usage: {{script}} notify <test> [OPTIONS] <source|update>

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
    --secrets-dir <path>                 (default: {{default_secrets_dir}})
    --help
    --help-full

Examples:
    {{script}} notify test --target onsite-usb homes
    {{script}} notify test --target offsite-storj --provider ntfy homes
    {{script}} notify test update --provider ntfy --dry-run
    {{script}} notify test --target onsite-usb --dry-run homes

Use --help-full for the detailed notify reference.
`,
		"{{default_secrets_dir}}", config.DefaultSecretsDir,
	)
}

func FullNotifyUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	return scriptTemplate(meta, `Usage: {{script}} notify <test> [OPTIONS] <source|update>

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
    --secrets-dir <path>    Override secrets directory (default: {{default_secrets_dir}})
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
      - uses the global app config at <config-dir>/{{app_config_file}}
      - does not require a label, target, or storage secrets
      - sends a synthetic update notification; default event is update_install_failed
      - bypasses update.notify.notify_on because it is an explicit operator test

PROVIDER SELECTION:
    --provider all          Test every configured destination for the selected target
    --provider webhook      Test only the configured webhook destination
    --provider ntfy         Test only the configured ntfy destination

EXAMPLES:
    {{script}} notify test --target onsite-usb homes
    {{script}} notify test --target offsite-storj --provider ntfy homes
    {{script}} notify test --target onsite-usb --severity critical homes
    {{script}} notify test --target onsite-usb --summary "NAS alert path test" --message "Synthetic end-to-end notification check" homes
    {{script}} notify test --target onsite-usb --dry-run homes
    {{script}} notify test --target onsite-usb --json-summary homes
    {{script}} notify test update --provider ntfy --event update_install_failed
    {{script}} notify test update --provider ntfy --severity critical --dry-run
`,
		"{{default_secrets_dir}}", config.DefaultSecretsDir,
		"{{app_config_file}}", config.DefaultAppConfigFile,
	)
}

func UpdateUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	return scriptTemplate(meta, `Usage: {{script}} update [OPTIONS]

Update options:
    --check-only
    --force
    --yes
    --keep <count>                       (default: {{default_keep}})
    --version <tag>                      (default: latest)
    --attestations <off|auto|required>   (default: off)
    --config-dir <path>                  (default: <binary-dir>/.config)
    --help
    --help-full

Examples:
    {{script}} update --check-only
    {{script}} update --yes
    {{script}} update --attestations required --yes
    {{script}} update --version v4.1.8 --yes
    {{script}} update --yes --config-dir /usr/local/lib/duplicacy-backup/.config

Use --help-full for the detailed update reference.
`,
		"{{default_keep}}", fmt.Sprint(updatepkg.DefaultKeep),
	)
}

func RollbackUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
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

func FullRollbackUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
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

func FullUpdateUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
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

func FullConfigUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	cfgDir := workflow.EffectiveConfigDir(rt)
	return scriptTemplate(meta, `Usage: {{script}} config <validate|explain|paths> [OPTIONS] <source>

CONFIG COMMANDS:
    validate                Validate the resolved config and configured secrets
    explain                 Show the resolved config values for the selected target
    paths                   Show the resolved stable config, source, log, and any applicable secrets paths

OPTIONS:
    --target <name>         Select the named target (required)
    --config-dir <path>     Override config directory (default: <binary-dir>/.config)
    --secrets-dir <path>    Override secrets directory (default: {{default_secrets_dir}})
    --help                  Show the concise help message
    --help-full             Show the detailed config help message

BEHAVIOUR:
    validate, explain, and paths operate on one selected target from a label config at a time.
    Every config command requires an explicit --target selection.

DEFAULT LOCATIONS:
    Config dir             : {{config_dir}}
    Secrets dir            : {{default_secrets_dir}}

EXAMPLES:
    {{script}} config validate --target onsite-usb homes
    {{script}} config validate --target offsite-storj homes
    {{script}} config explain --target onsite-usb homes
    {{script}} config explain --target offsite-storj homes
    {{script}} config paths --target onsite-usb homes
`,
		"{{default_secrets_dir}}", config.DefaultSecretsDir,
		"{{config_dir}}", cfgDir,
	)
}

func VersionText(meta workflow.Metadata) string {
	return fmt.Sprintf("%s %s (built %s)\n", meta.ScriptName, meta.Version, meta.BuildTime)
}
