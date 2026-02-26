package outputs_users

type AuthenticateOutput struct {
	Authenticated bool           `json:"authenticated"`
	User          map[string]any `json:"user"`
}
