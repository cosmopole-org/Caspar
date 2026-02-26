package packet

type Error struct {
	Message string `json:"message"`
}

func BuildErrorJson(message string) Error {
	return Error{Message: message}
}
