package command

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func ConfigUsageText(meta workflow.Metadata, rt workflow.Env) string {
	return scriptTemplate(meta, `Usage: {{script}} config <validate|explain|paths> [OPTIONS] <source>

Config commands:
    validate
    explain
    paths

Options:
    --target <name>
    --config-dir <path>     (default: $HOME/.config/duplicacy-backup)
    --secrets-dir <path>    (default: {{default_secrets_dir}})
    --help
    --help-full

Examples:
    sudo {{script}} config validate --target onsite-usb homes
    {{script}} config explain --target offsite-storj homes
    {{script}} config paths --target onsite-usb homes

Use --help-full for the detailed config reference.
`,
		"{{default_secrets_dir}}", defaultSecretsDirDisplay(rt),
	)
}

func FullConfigUsageText(meta workflow.Metadata, rt workflow.Env) string {
	cfgDir := workflow.EffectiveConfigDir(rt)
	return scriptTemplate(meta, `Usage: {{script}} config <validate|explain|paths> [OPTIONS] <source>

CONFIG COMMANDS:
    validate                Validate the resolved config and configured secrets
    explain                 Show the resolved config values for the selected target
    paths                   Show the resolved stable config, source, log, and any applicable secrets paths

OPTIONS:
    --target <name>         Select the named target (required)
    --config-dir <path>     Override config directory (default: $HOME/.config/duplicacy-backup)
    --secrets-dir <path>    Override secrets directory (default: {{default_secrets_dir}})
    --help                  Show the concise help message
    --help-full             Show the detailed config help message

BEHAVIOUR:
    validate, explain, and paths operate on one selected target from a label config at a time.
    Every config command requires an explicit --target selection.
    validate checks source path shape with stat/Btrfs probes; it does not require
    the operator user to read protected source contents. Backup execution still
    runs under sudo when root is needed for full source access and snapshots.

DEFAULT LOCATIONS:
    Config dir             : {{config_dir}}
    Secrets dir            : {{default_secrets_dir}}

EXAMPLES:
    sudo {{script}} config validate --target onsite-usb homes
    {{script}} config validate --target offsite-storj homes
    {{script}} config explain --target onsite-usb homes
    {{script}} config explain --target offsite-storj homes
    {{script}} config paths --target onsite-usb homes
`,
		"{{default_secrets_dir}}", defaultSecretsDirDisplay(rt),
		"{{config_dir}}", cfgDir,
	)
}
