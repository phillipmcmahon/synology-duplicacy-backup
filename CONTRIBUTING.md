# Contributing

Thank you for considering a contribution!

## Code Style

All Go source files **must** pass `gofmt` before being committed.  
The CI pipeline (`lint` job) enforces this — a PR will fail if any file is not formatted.

```bash
# Check for unformatted files
gofmt -l .

# Auto-fix all files
gofmt -w .

# Also run vet, Staticcheck, and tests before pushing
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck ./...
go test -race ./...
```

## Public Compatibility Contract

The public contract is the operator surface: CLI commands and flags, config
files, scheduler commands, restore/update behaviour, privilege model,
published packages, and operator-facing output. Go packages under `internal/`
are implementation detail; changing them does not require a major release
unless the operator contract changes too.

## Architecture Guards

`make validate` and GitHub validation run architecture and quality guards that
protect intentional design boundaries:

- `scripts/check-plan-section-boundary.sh` preserves the section-owned
  `workflow.Plan` shape. If you add, rename, or remove fields in
  `internal/workflow/plan.go`, update the guard's `FIELDS` list in the same
  change. When writing docs or comments about the old pre-section shape, avoid
  spelling literal retired selectors such as
  `Plan.<field-name-from-the-guard>`. Prefer phrases like "the previous flat
  Plan shape" so historical prose does not look like a live code regression to
  the lint.
- `scripts/check-coverage-floor.sh` enforces the current `85.0%` floor for
  every package with coverable statements and for aggregate coverage. It runs
  through `make validate` and GitHub Actions. If a change legitimately moves
  the target, update the script, `TESTING.md`, and release notes in the same
  commit so the quality bar stays explicit.

Coverage guard review decisions:

- The CI guard intentionally runs as a self-contained step after
  `go test -v -race ./...`, even though that means tests execute twice. The
  clarity and independence are preferred while the full workflow remains fast;
  optimize only if CI runtime becomes a real bottleneck.
- The guard currently runs on Ubuntu Go 1.26 only. If the workflow later adds
  macOS, Windows, or multiple Go-version test matrices, decide explicitly
  whether coverage should run on every matrix entry or only on the canonical
  Linux release-validation leg.
- The current floor is a single `85.0%` baseline. Package-specific floors for
  security-sensitive or historically higher-coverage packages are tracked as a
  deliberate follow-up in issue `#295`, not hidden inside the baseline guard.

## Pre-commit Hook (recommended)

A ready-made hook is provided in `scripts/pre-commit`.  
Install it once to catch formatting issues before they reach CI:

```bash
cp scripts/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

## Editor Configuration

An `.editorconfig` file is included so most editors (VS Code, GoLand, Vim, etc.)  
automatically use tabs for Go files and LF line endings.

## Pull Request Checklist

- [ ] Keep mechanical renames and behaviour changes in separate commits when
      practical, so review and git history can distinguish vocabulary churn
      from runtime or operator-facing changes
- [ ] `gofmt -l .` returns no output
- [ ] `go vet ./...` passes
- [ ] `go run honnef.co/go/tools/cmd/staticcheck ./...` passes
- [ ] `go test -race ./...` passes
- [ ] CHANGELOG.md updated (if user-facing change)
- [ ] Related issue and project board fields are current after each commit:
      issue state, `status:*` labels, GitHub project `Status`, and custom
      `Workflow` should tell the same story
- [ ] For project workflow changes, prefer
      `scripts/project-transition.sh --issue <number> --stage <stage>` over
      manual label and board edits
- [ ] For multiline GitHub issue bodies, use a Markdown body file with
      `gh issue create --body-file` or `gh issue edit --body-file`; do not use
      inline strings with escaped `\n`, because they render as literal text
