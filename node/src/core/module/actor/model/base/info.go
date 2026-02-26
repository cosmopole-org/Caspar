package base

import "strings"

type Info struct {
	isGod   bool
	userId  string
	pointId string
}

func NewInfo(userId string, pointId string) *Info {
	return &Info{isGod: false, userId: userId, pointId: pointId}
}

func NewGodInfo(userId string, pointId string, isGod bool) *Info {
	return &Info{isGod: isGod, userId: userId, pointId: pointId}
}

func (info *Info) IsGod() bool {
	return info.isGod
}

func (info *Info) UserId() string {
	return info.userId
}

func (info *Info) PointId() string {
	return info.pointId
}

func (info *Info) Identity() (string, string) {
	identity := strings.Split(info.userId, "@")
	if len(identity) == 2 {
		return identity[0], identity[1]
	}
	return "", ""
}