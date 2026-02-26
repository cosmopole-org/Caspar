package updates_points

import "kasper/src/shell/api/model"

type Delete struct {
	Point model.Point `json:"point"`
}
