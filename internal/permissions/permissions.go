// Package permissions handles local repository ownership and permission
// normalisation (chown/chmod) for local backup targets.
//
// The chown operation is executed via an [exec.Runner] so that tests can
// inject a [exec.MockRunner].  The chmod operations use [os.Chmod] directly
// since they do not require an external process.
package permissions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

// Fix normalises ownership and permissions on a local backup target.
// Directories get 770, files get 660.  The runner is used for the chown
// command; chmod is applied via [os.Chmod].
func Fix(runner execpkg.Runner, log *logger.Logger, target, owner, group string, dryRun bool) error {
	log.Info("Starting local ownership and permission normalisation on %s", target)
	log.PrintLine("Fix Perms Path", target)
	log.PrintLine("Fix Perms Owner", owner)
	log.PrintLine("Fix Perms Group", group)

	ownerGroup := fmt.Sprintf("%s:%s", owner, group)

	// chown -R owner:group target
	if dryRun {
		log.DryRun("chown -R %s %s", ownerGroup, target)
	} else {
		stdout, stderr, err := runner.Run(context.Background(), "chown", "-R", ownerGroup, target)
		if stdout != "" {
			fmt.Print(stdout)
		}
		if stderr != "" {
			fmt.Print(stderr)
		}
		if err != nil {
			return fmt.Errorf("failed to change ownership of %s: %w", target, err)
		}
	}

	if dryRun {
		log.DryRun("find %s -type d -exec chmod 770 {} +", target)
		log.DryRun("find %s -type f -exec chmod 660 {} +", target)
	} else {
		err := filepath.Walk(target, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return os.Chmod(path, 0770)
			}
			return os.Chmod(path, 0660)
		})
		if err != nil {
			return fmt.Errorf("failed to set permissions in %s: %w", target, err)
		}
	}

	log.Info("Completed local ownership and permission normalisation")
	return nil
}
