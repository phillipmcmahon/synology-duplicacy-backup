// Package permissions handles local repository ownership and permission
// normalisation (chown/chmod) for local backup targets.
//
// The chown operation is executed via an [exec.Runner] so that the platform
// owns user and group resolution, while tests can inject an [exec.MockRunner].
// The chmod walk skips symlinks so permission repair never follows a link and
// changes a target outside the repository tree.
//
// Functions return structured [errors.PermissionsError] values with rich
// context instead of logging directly.  The coordinator is responsible for
// all operator-facing output.
package permissions

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	apperrors "github.com/phillipmcmahon/synology-duplicacy-backup/internal/errors"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
)

// Fix normalises ownership and permissions on a local backup target.
// Directories get 770, files and other non-symlink entries get 660. Symlinks
// are not followed during ownership or permission repair.
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

	// chown -h -R applies ownership recursively without following symlinks.
	_, _, err := runner.Run(context.Background(), "chown", "-h", "-R", ownerGroup, target)
	if err != nil {
		return apperrors.NewPermissionsError("chown", fmt.Errorf("failed to change ownership: %w", err), "target", target, "owner", ownerGroup)
	}

	// Set directory (770) and file (660) permissions
	walkErr := filepath.WalkDir(target, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return os.Chmod(path, 0770)
		}
		return os.Chmod(path, 0660)
	})
	if walkErr != nil {
		return apperrors.NewPermissionsError("chmod", fmt.Errorf("failed to set permissions: %w", walkErr), "target", target)
	}

	return nil
}
