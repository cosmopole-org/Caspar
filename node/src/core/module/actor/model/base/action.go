package base

import (
	"kasper/src/abstract/models/action"
	"kasper/src/abstract/models/input"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/state"
)

type Func func(state state.IState, input input.IInput) (any, error)

type Action struct {
	modifier func(bool, func(trx.ITrx) error)
	key      string
	Func     Func
}

func NewAction(modifier func(bool, func(trx.ITrx) error), key string, fn Func) action.IAction {
	return &Action{modifier: modifier, key: key, Func: fn}
}

func (a *Action) StateModifier() func(bool, func(trx.ITrx) error) {
	return a.modifier
}

func (a *Action) Key() string {
	return a.key
}

func (a *Action) Act(state state.IState, input input.IInput) (int, any, error) {
	result, err := a.Func(state, input)
	if err != nil {
		return 0, nil, err
	}
	return 1, result, nil
}
