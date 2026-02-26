package outputs_machiner

import (
	"kasper/src/shell/api/model"
)

type CreateOutput struct {
	User model.User `json:"user"`
}
