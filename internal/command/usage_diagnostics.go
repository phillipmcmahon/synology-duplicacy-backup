package command

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func DiagnosticsUsageText(meta workflow.Metadata, rt workflow.Env) string {
	return scriptTemplate(meta, `Usage: {{script}} diagnostics [OPTIONS] <source>

Diagnostics options:
    --target <name>
    --json-summary
    --config-dir <path>     (default: $HOME/.config/duplicacy-backup)
    --secrets-dir <path>    (default: {{default_secrets_dir}})
    --help
    --help-full

Examples:
    {{script}} diagnostics --target onsite-usb homes
    {{script}} diagnostics --target offsite-storj --json-summary homes

Use --help-full for the detailed diagnostics reference.
`,
		"{{default_secrets_dir}}", defaultSecretsDirDisplay(rt),
	)
}

func FullDiagnosticsUsageText(meta workflow.Metadata, rt workflow.Env) string {
	cfgDir := workflow.EffectiveConfigDir(rt)
	return scriptTemplate(meta, `Usage: {{script}} diagnostics [OPTIONS] <source>

DIAGNOSTICS COMMAND:
    diagnostics             Print a redacted support bundle for one label and target

OPTIONS:
    --target <name>         Select the named target (required)
    --json-summary          Write the diagnostics report as machine-readable JSON
    --config-dir <path>     Override config directory (default: $HOME/.config/duplicacy-backup)
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
		"{{default_secrets_dir}}", defaultSecretsDirDisplay(rt),
		"{{config_dir}}", cfgDir,
		"{{state_dir}}", meta.StateDir,
		"{{log_dir}}", meta.LogDir,
	)
}
