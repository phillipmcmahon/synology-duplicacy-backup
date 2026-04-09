// Package permissions handles local repository ownership and permission
// normalisation (chown/chmod) for local backup targets.
//
// The chown operation is executed via an [exec.Runner] so that tests can
// inject a [exec.MockRunner].  The chmod operations use [os.Chmod] directly
// since they do not require an external process.
//
// Functions return structured [errors.PermissionsError] values with rich
// context instead of logging directly.  The coordinator is responsible for
// all operator-facing output.
package permissions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
)

// Fix normalises ownership and permissions on a local backup target.
// Directories get 770, files get 660.  The runner is used for the chown
// command; chmod is applied via [os.Chmod].
//
// In dry-run mode no commands are executed and nil is returned.
//
// Returns a [*errors.PermissionsError] on failure with context including
// the target path, owner, and group.
func Fix(runner execpkg.Runner, target, owner, group string, dryRun bool) error {
	if dryRun {
		return nil
	}

	ownerGroup := fmt.Sprintf("%s:%s", owner, group)

	// chown -R owner:group target
	_, _, err := runner.Run(context.Background(), "chown", "-R", ownerGroup, target)
	if err != nil {
		return apperrors.NewPermissionsError("chown", fmt.Errorf("failed to change ownership: %w", err), "target", target, "owner", ownerGroup)
	}

	// Set directory (770) and file (660) permissions
	walkErr := filepath.Walk(target, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.Chmod(path, 0770)
		}
		return os.Chmod(path, 0660)
	})
	if walkErr != nil {
		return apperrors.NewPermissionsError("chmod", fmt.Errorf("failed to set permissions: %w", walkErr), "target", target)
	}

	return nil
}
