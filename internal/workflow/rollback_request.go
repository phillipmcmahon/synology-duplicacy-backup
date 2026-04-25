package workflow

// RollbackRequest is the rollback command's narrowed view of CLI intent.
//
// The parser still returns Request while command-specific workflow models are
// introduced incrementally. Rollback code should use this type so it does not
// depend on unrelated restore, notify, health, or runtime fields.
type RollbackRequest struct {
	Command   string
	Version   string
	CheckOnly bool
	Yes       bool
}

func NewRollbackRequest(req *Request) RollbackRequest {
	if req == nil {
		return RollbackRequest{}
	}
	return RollbackRequest{
		Command:   req.RollbackCommand,
		Version:   req.RollbackVersion,
		CheckOnly: req.RollbackCheckOnly,
		Yes:       req.RollbackYes,
	}
}
