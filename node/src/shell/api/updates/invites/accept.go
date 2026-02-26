package updates_invites

import "kasper/src/shell/api/model"

type Accept struct {
	User  model.User `json:"user"`
	PointId string `json:"pointId"`
}
