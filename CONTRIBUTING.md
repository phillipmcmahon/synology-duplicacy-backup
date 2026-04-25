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

- [ ] `gofmt -l .` returns no output
- [ ] `go vet ./...` passes
- [ ] `go run honnef.co/go/tools/cmd/staticcheck ./...` passes
- [ ] `go test -race ./...` passes
- [ ] CHANGELOG.md updated (if user-facing change)
- [ ] Related issue and project board fields are current after each commit:
      issue state, `status:*` labels, GitHub project `Status`, and custom
      `Workflow` should tell the same story
