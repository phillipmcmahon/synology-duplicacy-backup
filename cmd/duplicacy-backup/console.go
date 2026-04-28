package main

import (
	"fmt"
	"os"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

func writeDirectInfo(format string, args ...interface{}) {
	writeDirectStderr(logger.INFO, format, args...)
}

func writeDirectWarn(format string, args ...interface{}) {
	writeDirectStderr(logger.WARNING, format, args...)
}

func writeDirectError(format string, args ...interface{}) {
	writeDirectStderr(logger.ERROR, format, args...)
}

func writeDirectStderr(level logger.Level, format string, args ...interface{}) {
	line := fmt.Sprintf("[%s] %s", level.String(), fmt.Sprintf(format, args...))
	fmt.Fprintln(os.Stderr, logger.ColourizeForLevel(level, line, logger.ColourEnabled(os.Stderr)))
}
