package packet

type Packet struct {
	Origin string `json:"origin"`
	Data   string `json:"data"`
}

type LogPacket struct {
	Id      string `json:"id"`
	PointId string `json:"pointId"`
	UserId  string `json:"userId"`
	Data    string `json:"data"`
	Time    int64  `json:"time"`
	Edited  bool   `json:"edited"`
}

type BuildPacket struct {
	Id        string `json:"id"`
	BuildId   string `json:"buildId"`
	MachineId string `json:"machineId"`
	Data      string `json:"data"`
}
