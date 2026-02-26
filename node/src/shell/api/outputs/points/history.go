package outputs_points

import "kasper/src/abstract/models/packet"

type HistoryOutput struct {
	Packets []packet.LogPacket `json:"packets"`
}
