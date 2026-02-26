package state

import (
	"kasper/src/abstract/models/info"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/state"
)

type State struct {
	info info.IInfo
	trx  trx.ITrx
	src  string
}

func (s *State) Info() info.IInfo {
	return s.info
}

func (s *State) SetInfo(i info.IInfo) {
	s.info = i
}

func (s *State) Trx() trx.ITrx {
	return s.trx
}

func (s *State) SetTrx(newTrx trx.ITrx) {
	s.trx = newTrx
}

func (s *State) Source() string {
	return s.src
}

func (s *State) SetSource(src string) {
	s.src = src
}

func NewState(args ...interface{}) state.IState {
	var t trx.ITrx
	if (len(args) > 1) && (args[1] != nil) {
		t = args[1].(trx.ITrx)
	} else {
		t = nil
	}

	if len(args) > 0 {
		if len(args) > 2 {
			return &State{info: args[0].(info.IInfo), trx: t, src: args[2].(string)}
		} else {
			return &State{info: args[0].(info.IInfo), trx: t, src: ""}
		}
	} else {
		return &State{info: nil, trx: t, src: ""}
	}
}
