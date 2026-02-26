package updates_points

import "kasper/src/shell/api/model"

type UpdateMember struct {
	PointId  string         `json:"pointId"`
	User     model.User     `json:"user"`
	Metadata map[string]any `json:"metadata"`
}
