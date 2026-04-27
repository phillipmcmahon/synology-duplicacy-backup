package workflow

import "github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"

type localStoragePolicy interface {
	UsesLocalDiskStorage() bool
}

// Path-based local repositories are root-protected by policy; non-root
// commands should stop before probing them through Duplicacy.
func localRepositoryRequiresSudo(cfg *config.Config, rt Runtime) bool {
	return localRepositoryRequiresSudoForStorage(cfg, rt)
}

func localRepositoryRequiresSudoForStorage(cfg localStoragePolicy, rt Runtime) bool {
	return cfg != nil && cfg.UsesLocalDiskStorage() && runtimeEUID(rt) != 0
}
