package model

type ElpisInput struct {
	Data string
}

func (d ElpisInput) GetSpaceId() string {
	return ""
}

func (d ElpisInput) GetTopicId() string {
	return ""
}

func (d ElpisInput) GetMemberId() string {
	return ""
}

func (d ElpisInput) Origin() string {
	return ""
}