package outputs_users

import (
	"kasper/src/shell/api/model"
)

type CreateOutput struct {
	User    model.User    `json:"user"`
	Session model.Session `json:"session"`
}
