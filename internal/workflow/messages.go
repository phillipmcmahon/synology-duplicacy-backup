package workflow

import (
	"fmt"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/operator"
)

type MessageError = operator.MessageError

// NewMessageError is retained as a runtime-package compatibility alias. New
// code should import internal/operator and call operator.NewMessageError.
var NewMessageError = operator.NewMessageError

// OperatorMessage is retained as a runtime-package compatibility alias. New
// code should import internal/operator and call operator.Message.
var OperatorMessage = operator.Message

func statusLinef(format string, args ...interface{}) string {
	return strings.TrimSpace(fmt.Sprintf(format, args...))
}
