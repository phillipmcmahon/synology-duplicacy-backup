#!/bin/sh

set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

FIELDS='Threads|Target|Location|BackupLabel|Filter|FilterLines|OwnerGroup|PruneOptions|PruneArgs|PruneArgsDisplay|LocalOwner|LocalGroup|LogRetentionDays|SafePruneMaxDeletePercent|SafePruneMaxDeleteCount|SafePruneMinTotalForPercent|RunTimestamp|SnapshotSource|SnapshotTarget|RepositoryPath|WorkRoot|DuplicacyRoot|BackupTarget|ConfigDir|ConfigFile|SecretsDir|SecretsFile|ModeDisplay|SnapshotCreateCommand|SnapshotDeleteCommand|WorkDirCreateCommand|PreferencesWriteCommand|FiltersWriteCommand|WorkDirDirPermsCommand|WorkDirFilePermsCommand|BackupCommand|ValidateRepoCommand|PrunePreviewCommand|PolicyPruneCommand|CleanupStorageCommand|WorkDirRemoveCommand|DoBackup|DoPrune|DoCleanupStore|ForcePrune|DryRun|Verbose|JSONSummary|NeedsDuplicacySetup|NeedsSnapshot|DefaultNotice|OperationMode|SourcePath|StorageURL|SnapshotID'
DIRECT_PATTERN="(^|[^[:alnum:]_])(plan|p)\\.(${FIELDS})([^[:alnum:]_]|$)"
DOC_PATTERN="Plan\\.(${FIELDS})([^[:alnum:]_]|$)"

failures=0
direct_hits="$(mktemp)"
doc_hits="$(mktemp)"
trap 'rm -f "$direct_hits" "$doc_hits"' EXIT

cd "$ROOT"

find internal cmd -type f -name '*.go' -exec grep -EnH "$DIRECT_PATTERN" {} + >"$direct_hits" || true
find . -type f \( -name '*.go' -o -name '*.md' -o -name '*.sh' \) \
    ! -path './.git/*' \
    -exec grep -EnH "$DOC_PATTERN" {} + >"$doc_hits" || true

if [ -s "$direct_hits" ]; then
    echo "Plan section boundary check failed: possible direct flat-field access on a Plan value." >&2
    echo "Use plan.Request.*, plan.Config.*, plan.Paths.*, or plan.Display.* instead." >&2
    cat "$direct_hits" >&2
    failures=1
fi

if [ -s "$doc_hits" ]; then
    echo "Plan section boundary check failed: literal Plan.<old-field> references remain." >&2
    echo "Update docs, comments, or scripts to the section-owned Plan shape." >&2
    cat "$doc_hits" >&2
    failures=1
fi

if [ "$failures" -ne 0 ]; then
    exit 1
fi

echo "Plan section boundary check passed"
