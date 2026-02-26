package outputs_points

import (
	"kasper/src/shell/api/model"
)

type AdminPoiint struct {
	model.Point
	Admin bool `json:"admin"`
}

type CreateOutput struct {
	Point AdminPoiint `json:"point"`
}
