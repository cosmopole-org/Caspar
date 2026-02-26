package inputs_invites

import "kasper/src/shell/utils/origin"

type ListPointInvitesInput struct {
	PointId string `json:"pointId" validate:"required"`
}

func (d ListPointInvitesInput) GetData() any {
	return "dummy"
}

func (d ListPointInvitesInput) GetPointId() string {
	return d.PointId
}

func (d ListPointInvitesInput) Origin() string {
	return origin.FindOrigin(d.PointId)
}
