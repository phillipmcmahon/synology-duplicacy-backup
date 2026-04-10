//go:build !linux && !darwin

package logger

func isTerminalFD(fd uintptr) bool {
	return false
}
