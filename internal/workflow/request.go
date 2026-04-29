package workflow

import "github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"

type RequestError = workflowcore.RequestError
type Request = workflowcore.Request

var NewRequestError = workflowcore.NewRequestError
var NewUsageRequestError = workflowcore.NewUsageRequestError
