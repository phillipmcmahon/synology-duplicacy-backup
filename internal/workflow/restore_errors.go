package workflow

import "errors"

// ErrRestoreCancelled means the operator deliberately exited before restore
// execution, for example by typing q or answering no. It is a clean exit and
// dispatch maps it to exit code 0.
var ErrRestoreCancelled = errors.New("restore cancelled")

// ErrRestoreInterrupted means an active restore process was interrupted after
// execution started. Dispatch maps it to exit code 1 because the drill
// workspace may contain a partial restore that needs inspection.
var ErrRestoreInterrupted = errors.New("restore interrupted")
