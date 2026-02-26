package signaler

import cmap "github.com/orcaman/concurrent-map/v2"

type ISignaler interface {
	Lock()
	Unlock()
	Listeners() *cmap.ConcurrentMap[string, *Listener]
	Groups() *cmap.ConcurrentMap[string, *Group]
	ListenToSingle(listener *Listener)
	ListenToGroup(listener *Listener, overrideFunctionaly bool)
	BrdigeGlobally(listener *GlobalListener, overrideFunctionaly bool)
	ListenToJoin(listener *JoinListener)
	SignalUser(key string, listenerId string, data any, pack bool)
	SignalGroup(key string, groupId string, data any, pack bool, exceptions []string)
	JoinGroup(groupId string, userId string)
	LeaveGroup(groupId string, userId string)
	RetriveGroup(groupId string) (*Group, bool)
}

type Group struct {
	Points   *cmap.ConcurrentMap[string, string]
	Listener *Listener
	Override bool
}

type Listener struct {
	Id      string
	Paused  bool
	DisTime int64
	Signal  func(string, any)
}

type GlobalListener struct {
	Signal func(string, any)
}

type JoinListener struct {
	Join  func(string, string)
	Leave func(string, string)
}
