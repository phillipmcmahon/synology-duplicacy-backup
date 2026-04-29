package restore

import (
	"bufio"
	"os"
)

type restoreLineReader interface {
	ReadString(delim byte) (string, error)
}

func runtimeStdinReader(rt Runtime) (*bufio.Reader, bool) {
	if rt.StdinIsTTY == nil || !rt.StdinIsTTY() {
		return nil, false
	}
	stdin := os.Stdin
	if rt.Stdin != nil && rt.Stdin() != nil {
		stdin = rt.Stdin()
	}
	return bufio.NewReader(stdin), true
}
