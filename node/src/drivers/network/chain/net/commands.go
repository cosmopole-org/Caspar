package net

import (
	"kasper/src/drivers/network/chain/hashgraph"
	"kasper/src/drivers/network/chain/peers"
)

// SyncRequest corresponds to  the pull part of the pull-push gossip protocol.
// It is used to retrieve unknown Events from another node. The Known map
// represents how much the requester currently knows about the hashgraph. The
// SyncLimit indicates the max number of Events to include in the response.
type SyncRequest struct {
	FromID       uint32
	Known        map[uint32]int
	SyncLimit    int
	WorkChainId  string
	ShardChainId string
}

// SyncResponse returns a list of Events as requested by a SyncRequest. The
// known map indicates how much the responder knows about the hashgraph. Events
// are encoded in light-weight wire format to take less space.
type SyncResponse struct {
	FromID uint32
	Events []hashgraph.WireEvent
	Known  map[uint32]int
	WorkChainId  string
	ShardChainId string
}

// EagerSyncRequest corresponds to the push part of the pull-push gossip
// protocol. It is used to actively push Events to a node without it being
// requested.
type EagerSyncRequest struct {
	FromID uint32
	Events []hashgraph.WireEvent
	WorkChainId  string
	ShardChainId string
}

// EagerSyncResponse indicates the success or failure of an EagerSyncRequest.
type EagerSyncResponse struct {
	FromID  uint32
	Success bool
	WorkChainId  string
	ShardChainId string
}

// FastForwardRequest is used to request a Block, Frame, and Snapshot, from
// which to fast-forward.
type FastForwardRequest struct {
	FromID uint32
	WorkChainId  string
	ShardChainId string
}

// FastForwardResponse encapsulates the response to a FastForwardRequest.
type FastForwardResponse struct {
	FromID   uint32
	Block    hashgraph.Block
	Frame    hashgraph.Frame
	Snapshot []byte
	WorkChainId  string
	ShardChainId string
}

// JoinRequest is used to submit an InternalTransaction to join a Babble group.
type JoinRequest struct {
	InternalTransaction hashgraph.InternalTransaction
	WorkChainId  string
	ShardChainId string
}

// JoinResponse contains the response to a JoinRequest.
type JoinResponse struct {
	FromID        uint32
	Accepted      bool
	AcceptedRound int
	Peers         []*peers.Peer
	WorkChainId  string
	ShardChainId string
}
