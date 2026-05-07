package displaytime

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

var readLocaltimeLink = os.Readlink
var loadTimeLocation = time.LoadLocation

// Location returns the timezone to use for human-readable operator output.
func Location() *time.Location {
	if location := systemLocaltimeLocation(); location != nil {
		return location
	}
	return time.Local
}

func systemLocaltimeLocation() *time.Location {
	target, err := readLocaltimeLink("/etc/localtime")
	if err != nil {
		return nil
	}
	const marker = "/zoneinfo/"
	index := strings.LastIndex(target, marker)
	if index < 0 {
		return nil
	}
	name := filepath.ToSlash(target[index+len(marker):])
	if name == "" {
		return nil
	}
	location, err := loadTimeLocation(name)
	if err != nil {
		return nil
	}
	return location
}
