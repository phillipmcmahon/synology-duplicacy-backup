package workflow

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
)

type localStoragePolicy interface {
	UsesRootProtectedLocalRepository() bool
}

// Privilege policy has two intentionally separate enforcement points:
// cmd/duplicacy-backup rejects ambiguous direct-root profile resolution before
// dispatch, while workflow rejects non-root access to root-protected local
// filesystem repositories at the command boundary that is about to probe
// storage.
// Keep these rules separate: one protects profile selection, the other protects
// root-owned repository metadata and chunks.

// Local filesystem repositories are root-protected by policy; non-root commands
// should stop before probing them through Duplicacy. Path-based remote mounts
// are not blocked here because their access is governed by the mount
// credentials and permissions.
func localRepositoryRequiresSudo(cfg *config.Config, rt Env) bool {
	return localRepositoryRequiresSudoForStorage(cfg, rt)
}

func localRepositoryRequiresSudoForStorage(cfg localStoragePolicy, rt Env) bool {
	return cfg != nil && cfg.UsesRootProtectedLocalRepository() && EnvEUID(rt) != 0
}

func restoreStorageRequiresSudo(plan *Plan, storage string) bool {
	return plan != nil && plan.Config.Location == locationLocal && duplicacy.NewStorageSpec(storage).IsLocalPath()
}
