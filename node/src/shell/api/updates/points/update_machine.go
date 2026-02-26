package updates_points

import "kasper/src/shell/api/model"

type UpdateApp struct {
	PointId string    `json:"pointId"`
	App     model.App `json:"app"`
	Machine Fn        `json:"machine"`
}
