package main

import (
	"fmt"
	"os"
)

var isSynologyDSM = detectSynologyDSM

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
