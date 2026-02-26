package chain

import "kasper/src/abstract/models/update"

type ChainMessage struct {
	Key        string
	Author     string
	Submitter  string
	Payload    []byte
	Signatures []string
	RequestId  string
	Recievers  []string
	ReplyTo    string
}

type ChainBaseRequest struct {
	Key        string
	Author     string
	Submitter  string
	Payload    []byte
	Signatures []string
	RequestId  string
	Tag        string
}

type ChainResponse struct {
	Executor  string
	Payload   []byte
	Signature string
	RequestId string
	Effects   Effects
	ResCode   int
	Err       string
	Tag       string
	ToUserId  string
}

type ChainAppletRequest struct {
	MachineId  string
	Key        string
	Author     string
	Submitter  string
	Payload    []byte
	Signatures []string
	RequestId  string
	Runtime    string
	Tag        string
	TokenId    string
}

type ChainElectionPacket struct {
	Type    string
	Key     string
	Meta    map[string]any
	Payload []byte
}

type Election struct {
	MyNum        string
	Participants map[string]bool
	Commits      map[string][]byte
	Reveals      map[string]string
}

type ChainCallback struct {
	Fn        func([]byte, int, error)
	Executors map[string]bool
	Responses map[string]string
	Tag       string
}

type MessageCallback struct {
	Id string
	Fn func(string, []byte)
}

type Effects struct {
	DbUpdates []update.Update `json:"dbUpdates"`
}
