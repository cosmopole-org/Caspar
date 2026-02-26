package outputs_points

import (
	"kasper/src/shell/api/model"
	updates_points "kasper/src/shell/api/updates/points"
)

type ListPointAppsOutput struct {
	Machines map[string]*updates_points.Fn `json:"machines"`
	Apps     map[string]model.App          `json:"apps"`
}
