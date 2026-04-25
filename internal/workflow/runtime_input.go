package workflow

import (
	"bufio"
	"os"
)

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
