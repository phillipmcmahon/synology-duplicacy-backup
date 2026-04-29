package restore

import (
	"fmt"
	"os"
)

var profileChown = os.Chown

func ChownProfilePath(meta Metadata, path string) error {
	if !meta.HasProfileOwner {
		return nil
	}
	if err := profileChown(path, meta.ProfileOwnerUID, meta.ProfileOwnerGID); err != nil {
		return fmt.Errorf("failed to set profile ownership on %s to %d:%d: %w", path, meta.ProfileOwnerUID, meta.ProfileOwnerGID, err)
	}
	return nil
}
