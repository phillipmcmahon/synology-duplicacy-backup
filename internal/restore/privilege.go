package restore

import "github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"

type localStoragePolicy interface {
	UsesRootProtectedLocalRepository() bool
}

func localRepositoryRequiresSudoForStorage(cfg localStoragePolicy, rt Runtime) bool {
	return cfg != nil && cfg.UsesRootProtectedLocalRepository() && restoreRuntimeEUID(rt) != 0
}

func restoreStorageRequiresSudo(plan *Plan, storage string) bool {
	return plan != nil && plan.Config.Location == "local" && duplicacy.NewStorageSpec(storage).IsLocalPath()
}

func restoreRuntimeEUID(rt Runtime) int {
	if rt.Geteuid != nil {
		return rt.Geteuid()
	}
	return 0
}
