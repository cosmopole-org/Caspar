package inputs_invites

type ListUserInvitesInput struct{}

func (d ListUserInvitesInput) GetData() any {
	return "dummy"
}

func (d ListUserInvitesInput) GetPointId() string {
	return ""
}

func (d ListUserInvitesInput) Origin() string {
	return ""
}
