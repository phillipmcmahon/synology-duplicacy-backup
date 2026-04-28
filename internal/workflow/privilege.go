package workflow

import "github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"

type localStoragePolicy interface {
	UsesLocalDiskStorage() bool
}

// Privilege policy has two intentionally separate enforcement points:
// cmd/duplicacy-backup rejects ambiguous direct-root profile resolution before
// dispatch, while workflow rejects non-root access to protected path-based
// local repositories at the command boundary that is about to probe storage.
// Keep these rules separate: one protects profile selection, the other protects
// root-owned repository metadata and chunks.

// Path-based local repositories are root-protected by policy; non-root
// commands should stop before probing them through Duplicacy.
func localRepositoryRequiresSudo(cfg *config.Config, rt Runtime) bool {
	return localRepositoryRequiresSudoForStorage(cfg, rt)
}

func localRepositoryRequiresSudoForStorage(cfg localStoragePolicy, rt Runtime) bool {
	return cfg != nil && cfg.UsesLocalDiskStorage() && runtimeEUID(rt) != 0
}
