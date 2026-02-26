package outputs_users

import (
	"kasper/src/shell/api/model"
)

type LoginOutput struct {
	User       model.User    `json:"user"`
	Session    model.Session `json:"session"`
	PrivateKey string        `json:"privateKey"`
}
