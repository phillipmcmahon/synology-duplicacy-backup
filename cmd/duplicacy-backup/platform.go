package main

import (
	"fmt"
	"os"
)

// isSynologyDSM is stubbable for tests; production uses detectSynologyDSM.
var isSynologyDSM = detectSynologyDSM

// detectSynologyDSM checks both common DSM marker files. Any stat failure is
// treated as non-DSM so operational commands fail closed on unknown platforms.
func detectSynologyDSM() bool {
	for _, path := range []string{"/etc/synoinfo.conf", "/etc.defaults/VERSION"} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func requireSynologyDSM() error {
	if isSynologyDSM() {
		return nil
	}
	return fmt.Errorf("duplicacy-backup requires Synology DSM with btrfs-backed /volume* storage; detected non-Synology environment")
}
