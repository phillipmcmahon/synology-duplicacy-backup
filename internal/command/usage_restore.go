package command

import (
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

const restoreUsageLine = "{{script}} restore <plan|list-revisions|run|select> [OPTIONS] <source>"

var restoreCommandHelpLines = []string{
	"    select                 Choose a restore point, inspect it, or build a tree-based restore selection; confirm to restore into a drill workspace",
	"    plan                   Resolve a safe read-only restore drill plan for one label and target",
	"    list-revisions         List visible backup revisions without executing a restore",
	"    run                    Prepare or reuse a drill workspace, then restore a revision, file, or pattern only there",
}

var restoreShortCommandHelpLines = []string{
	"    select                  guided operator flow",
	"    plan                    expert: explain resolved restore context",
	"    list-revisions          expert: list backup revisions",
	"    run                     expert: prepare a drill workspace and restore only there",
}

func restoreCommandHelpBlock() string {
	return strings.Join(restoreCommandHelpLines, "\n")
}

func restoreShortCommandHelpBlock() string {
	return strings.Join(restoreShortCommandHelpLines, "\n")
}

func restoreUsage(meta workflow.Metadata) string {
	return strings.ReplaceAll(restoreUsageLine, "{{script}}", meta.ScriptName)
}

func RestoreUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	return scriptTemplate(meta, `Usage: {{restore_usage}}

Restore commands:
{{restore_commands}}

Options:
    --target <name>
    --workspace <path>      override the derived drill workspace
    --revision <id>         required for run
    --path <path>           optional snapshot-relative path or pattern for run
    --path-prefix <path>    select only; start the tree picker under a useful subtree
    --limit <count>         list-revisions default: 50
    --dry-run               run only; print planned restore without executing
    --yes                   run only; skip interactive confirmation
    --json-summary          list-revisions only
    --config-dir <path>     (default: <binary-dir>/.config)
    --secrets-dir <path>    (default: {{default_secrets_dir}})
    --help
    --help-full

Interactive restore select:
    Choose q at a restore prompt or in the tree picker to cancel cleanly before execution.
    During an active restore, Ctrl-C cancels the running Duplicacy process,
    keeps the drill workspace, does not delete restored files, and reports
    progress.

Examples:
    {{script}} restore select --target onsite-usb homes
    {{script}} restore select --target onsite-usb --path-prefix phillipmcmahon/code homes
    {{script}} restore plan --target onsite-usb homes
    {{script}} restore list-revisions --target onsite-usb homes
    {{script}} restore run --target onsite-usb --revision 2403 --path docs/readme.md --yes homes
    {{script}} restore plan --target offsite-storj homes

Use --help-full for the detailed restore reference.
`,
		"{{restore_usage}}", restoreUsage(meta),
		"{{restore_commands}}", restoreShortCommandHelpBlock(),
		"{{default_secrets_dir}}", config.DefaultSecretsDir,
	)
}

func FullRestoreUsageText(meta workflow.Metadata, rt workflow.Runtime) string {
	cfgDir := workflow.EffectiveConfigDir(rt)
	return scriptTemplate(meta, `Usage: {{restore_usage}}

RESTORE COMMANDS:
{{restore_commands}}

OPTIONS:
    --target <name>        Select the named target (required)
    --workspace <path>     Override derived drill workspace; honored exactly when supplied
    --revision <id>        Required for run
    --path <path>          Optional snapshot-relative path or pattern for run
    --path-prefix <path>   select only; start browsing under a snapshot-relative prefix
    --limit <count>        Limit listed revisions (default: 50)
    --dry-run              run only; print planned restore without executing
    --yes                  run only; skip interactive confirmation
    --json-summary         list-revisions only; write machine-readable output
    --config-dir <path>    Override config directory (default: <binary-dir>/.config)
    --secrets-dir <path>   Override secrets directory (default: {{default_secrets_dir}})
    --help                 Show the concise restore help message
    --help-full            Show the detailed restore help message

BEHAVIOUR:
    operator default:
      - start with restore select for most live operator restores
      - choose a restore point first, then inspect only or restore
      - for restore actions, review the exact restore commands before confirming
    restore plan:
      - reads the selected label config
      - shows the resolved source path, storage value, config file, and applicable secrets file
      - reads the target state file when available to show the latest known backup revision
      - prints Duplicacy commands for creating a separate drill workspace, listing revisions, and restoring manually
      - does not create directories, write Duplicacy preferences, execute duplicacy restore, or copy data back
    restore list-revisions:
      - creates a temporary Duplicacy workspace unless --workspace is supplied
      - requires --workspace to point at a workspace containing .duplicacy/preferences when supplied
      - runs duplicacy list and prints visible revisions
      - does not run duplicacy restore or copy data back
    restore run:
      - requires --revision <id>
      - when --workspace is omitted, derives a predictable workspace from label, target, restore-point timestamp, and revision id
      - creates that workspace and writes .duplicacy/preferences when needed
      - rejects the live source path, source-child workspaces, and non-empty unprepared workspaces
      - runs duplicacy restore only from the drill workspace
      - uses --path for selective file restores or directory patterns
      - uses --yes for non-interactive execution
      - prints coloured status/progress to stderr during workspace preparation and restore execution
      - writes the final restore report to stdout, with compact Duplicacy totals on success and emitted diagnostics on failure
      - never restores over the live source path and never copies data back
    restore select:
      - requires an interactive terminal
      - is the primary operator restore flow
      - guides restore-point selection before offering inspect-only, full restore, or tree-based selective restore
      - uses arrow keys to move through the snapshot tree
      - uses Right to expand directories and Left to collapse them
      - uses Space to select or clear the current file or subtree
      - uses Tab to switch between the tree and the primitive detail pane
      - uses g to continue with the current selection and generate the restore commands
      - uses q to cancel selection or quit inspection
      - accepts q at text prompts to cancel cleanly before execution
      - accepts "--path-prefix <path>" to start browsing from a useful subtree
      - shows the exact restore run primitives that the current selection compiles to
      - inspect-only remains read-only and does not run duplicacy restore
      - for restore actions, shows the generated restore commands and asks for confirmation
      - shows listing progress before the picker and restore progress after confirmation
      - delegates restore actions to restore run, which prepares the workspace when needed
      - when --workspace is omitted for restore actions, uses a drill workspace named from the selected restore point, for example <label>-<target>-<restore-point-timestamp>-rev<id>
      - never copies data back

    expert path:
      - use restore plan, restore list-revisions, and restore run for explicit, scriptable, or incident-runbook-driven recovery
      - restore run self-prepares a predictable drill workspace when --workspace is omitted
      - pass --workspace only when you need a specific operator-chosen destination

    cancellation:
      - use q at restore select prompts or inside the picker to leave without restoring
      - answering no at the final confirmation prints the generated command report and exits without restoring
      - during an active restore, Ctrl-C cancels Duplicacy, keeps the drill workspace, does not delete restored files, and reports progress
      - cancelling before confirmation does not prepare a workspace or run duplicacy restore

DEFAULT LOCATIONS:
    Config dir             : {{config_dir}}
    Secrets dir            : {{default_secrets_dir}}

SAFETY MODEL:
    Restore drills should restore into a separate workspace first. Inspect the
    restored data there, then copy back deliberately with rsync --dry-run before
    any live write. See docs/restore-drills.md for the full procedure.

EXAMPLES:
    {{script}} restore select --target onsite-usb homes
    {{script}} restore select --target onsite-usb --path-prefix phillipmcmahon/code homes
    {{script}} restore plan --target onsite-usb homes
    {{script}} restore plan --target offsite-storj homes
    {{script}} restore list-revisions --target onsite-usb homes
    {{script}} restore list-revisions --target offsite-storj --json-summary homes
    {{script}} restore run --target onsite-usb --revision 2403 --path docs/readme.md --dry-run homes
    {{script}} restore run --target onsite-usb --revision 2403 --path 'docs/*' --yes homes
    {{script}} restore plan --target offsite-storj --config-dir /opt/etc --secrets-dir /opt/secrets homes
`,
		"{{restore_usage}}", restoreUsage(meta),
		"{{restore_commands}}", restoreCommandHelpBlock(),
		"{{default_secrets_dir}}", config.DefaultSecretsDir,
		"{{config_dir}}", cfgDir,
	)
}
