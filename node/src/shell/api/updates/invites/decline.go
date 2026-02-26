package updates_invites

import "kasper/src/shell/api/model"

type Decline struct {
	User    model.User `json:"user"`
	PointId string     `json:"pointId"`
}
