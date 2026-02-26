package module_actor

import (
	"kasper/src/abstract/models/action"
)

type Actor struct {
	actionMap map[string]action.IAction
}

func NewActor() *Actor {
	return &Actor{actionMap: make(map[string]action.IAction)}
}

func (a *Actor) InjectService(service interface{}) {
}

func (a *Actor) InjectAction(action action.IAction) {
	a.actionMap[action.Key()] = action
}

func (a *Actor) FetchAction(key string) action.IAction {
	return a.actionMap[key]
}
