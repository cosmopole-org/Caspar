package info

type IInfo interface {
	IsGod() bool
	UserId() string
	PointId() string
	Identity() (string, string)
}
