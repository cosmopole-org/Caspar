package updates_points

import models "kasper/src/shell/api/model"

type Send struct {
	Id     string       `json:"id"`
	User   models.User  `json:"user"`
	Point  models.Point `json:"point"`
	Action string       `json:"action"`
	Data   string       `json:"data"`
	Time   int64        `json:"time"`
	IsTemp bool         `json:"isTemp"`
}
