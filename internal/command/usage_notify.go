package command

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func NotifyUsageText(meta workflow.Metadata, rt workflow.Env) string {
	return scriptTemplate(meta, `Usage: {{script}} notify <test> [OPTIONS] <source|update>

Notify commands:
    test

Options:
    --storage <name>
    --provider <all|webhook|ntfy>        (default: all)
    --severity <warning|critical|info>   (default: warning)
    --event <name>
    --summary <text>
    --message <text>
    --dry-run
    --json-summary
    --config-dir <path>                  (default: $HOME/.config/duplicacy-backup)
    --secrets-dir <path>                 (default: {{default_secrets_dir}})
    --help
    --help-full

Examples:
    {{script}} notify test --storage onsite-usb homes
    {{script}} notify test --storage offsite-storj --provider ntfy homes
    {{script}} notify test update --provider ntfy --dry-run
    {{script}} notify test --storage onsite-usb --dry-run homes

Use --help-full for the detailed notify reference.
`,
		"{{default_secrets_dir}}", defaultSecretsDirDisplay(rt),
	)
}

func FullNotifyUsageText(meta workflow.Metadata, rt workflow.Env) string {
	return scriptTemplate(meta, `Usage: {{script}} notify <test> [OPTIONS] <source|update>

NOTIFY COMMANDS:
    test                    Send a synthetic test notification through the configured providers for the selected label and storage
    test update             Send a synthetic update notification through the global update notification config

OPTIONS:
    --storage <name>         Select the configured storage entry to test (required for label storage tests)
    --provider <name>       One of all, webhook, or ntfy (default: all)
    --severity <level>      One of warning, critical, or info (default: warning)
    --event <name>          Update event to simulate for notify test update
    --summary <text>        Override the default test summary line
    --message <text>        Override the default synthetic message body
    --dry-run               Preview the resolved destinations and synthetic payload without sending
    --json-summary          Write a machine-readable test summary to stdout
    --config-dir <path>     Override config directory (default: $HOME/.config/duplicacy-backup)
    --secrets-dir <path>    Override secrets directory (default: {{default_secrets_dir}})
    --help                  Show the concise notify help message
    --help-full             Show the detailed notify help message

BEHAVIOUR:
    notify test:
      - uses the existing label and storage config
      - uses storage-scoped notification auth tokens when configured
      - sends a clearly marked synthetic notification
      - bypasses notify_on / send_for gating because it is an explicit operator test
      - fails if the selected provider is not configured for the selected storage

    notify test update:
      - uses the global app config at <config-dir>/{{app_config_file}}
      - does not require a label, storage selection, or storage secrets
      - sends a synthetic update notification; default event is update_install_failed
      - bypasses update.notify.notify_on because it is an explicit operator test

PROVIDER SELECTION:
    --provider all          Test every configured destination for the selected storage
    --provider webhook      Test only the configured webhook destination
    --provider ntfy         Test only the configured ntfy destination

EXAMPLES:
    {{script}} notify test --storage onsite-usb homes
    {{script}} notify test --storage offsite-storj --provider ntfy homes
    {{script}} notify test --storage onsite-usb --severity critical homes
    {{script}} notify test --storage onsite-usb --summary "NAS alert path test" --message "Synthetic end-to-end notification check" homes
    {{script}} notify test --storage onsite-usb --dry-run homes
    {{script}} notify test --storage onsite-usb --json-summary homes
    {{script}} notify test update --provider ntfy --event update_install_failed
    {{script}} notify test update --provider ntfy --severity critical --dry-run
`,
		"{{default_secrets_dir}}", defaultSecretsDirDisplay(rt),
		"{{app_config_file}}", config.DefaultAppConfigFile,
	)
}
