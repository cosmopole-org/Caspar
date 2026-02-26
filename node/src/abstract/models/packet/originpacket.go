package packet

type OriginPacket struct {
	Type       string
	Key        string
	UserId     string
	PointId    string
	RequestId  string
	ResCode    int
	Binary     []byte
	Signature  string
	Exceptions []string
}
