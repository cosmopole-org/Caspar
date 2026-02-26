package proxy

import (
	"kasper/src/drivers/network/chain/hashgraph"
	"kasper/src/drivers/network/chain/node/state"
)

// AppProxy defines the interface which is used by Babble to communicate with
// the App
type AppProxy interface {
	SubmitCh() chan []byte
	CommitBlock(block hashgraph.Block) (CommitResponse, error)
	GetSnapshot(blockIndex int) ([]byte, error)
	Restore(snapshot []byte) error
	OnStateChanged(state.State) error
}
