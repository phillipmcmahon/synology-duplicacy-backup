package workflow

// UpdateRequest is the update command's narrowed view of CLI intent.
//
// The parser still returns Request while command-specific workflow models are
// introduced incrementally. Update code should use this type so it does not
// depend on unrelated restore, notify, health, or runtime fields.
type UpdateRequest struct {
	Command      string
	ConfigDir    string
	Version      string
	Keep         int
	Attestations string
	CheckOnly    bool
	Yes          bool
	Force        bool
}

func NewUpdateRequest(req *Request) UpdateRequest {
	if req == nil {
		return UpdateRequest{}
	}
	return UpdateRequest{
		Command:      req.UpdateCommand,
		ConfigDir:    req.ConfigDir,
		Version:      req.UpdateVersion,
		Keep:         req.UpdateKeep,
		Attestations: req.UpdateAttestations,
		CheckOnly:    req.UpdateCheckOnly,
		Yes:          req.UpdateYes,
		Force:        req.UpdateForce,
	}
}
