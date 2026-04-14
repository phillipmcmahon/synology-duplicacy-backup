## Highlights
- CLI coverage is now much stronger at the real entrypoint. The `cmd/duplicacy-backup` test suite now exercises `notify test` through the top-level command path and covers logger-initialisation failure handling for both health and runtime commands.
- Optional notification-token parsing is now simpler and better covered. The shared `internal/secrets` token-loading path now handles missing targets, missing tokens, uppercase keys, malformed TOML, and unknown keys through focused behavioural tests rather than duplicated parsing logic.
- This release materially improves confidence in operator-facing behaviour without changing the shipped command surface, which makes it a clean patch release focused on reliability and regression protection.

## Validation
- Linux Go 1.26: `go test ./...`
- Linux Go 1.26: `go vet ./...`
- Linux Go 1.26: `go test -cover ./...`

## Coverage
- Linux Go 1.26: overall coverage = `83.9%`
- Linux Go 1.26: `cmd/duplicacy-backup` coverage = `94.7%`
- Linux Go 1.26: `internal/workflow` coverage = `82.8%`
- Linux Go 1.26: `internal/duplicacy` coverage = `81.2%`
- Linux Go 1.26: `internal/config` coverage = `84.4%`
- Linux Go 1.26: `internal/secrets` coverage = `81.8%`
