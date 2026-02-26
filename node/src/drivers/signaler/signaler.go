package signaler

import (
	"encoding/json"
	"kasper/src/abstract/adapters/network"
	"kasper/src/abstract/adapters/signaler"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/trx"
	"log"
	"strings"
	"sync"

	cmap "github.com/orcaman/concurrent-map/v2"
)

type Signaler struct {
	lock           sync.Mutex
	app            core.ICore
	listeners      *cmap.ConcurrentMap[string, *signaler.Listener]
	groups         *cmap.ConcurrentMap[string, *signaler.Group]
	GlobalBridge   *signaler.GlobalListener
	LGroupDisabled bool
	JListener      *signaler.JoinListener
	Federation     network.IFederation
}

func (p *Signaler) Listeners() *cmap.ConcurrentMap[string, *signaler.Listener] {
	return p.listeners
}

func (p *Signaler) Groups() *cmap.ConcurrentMap[string, *signaler.Group] {
	return p.groups
}

func (p *Signaler) Lock() {
	p.lock.Lock()
}

func (p *Signaler) Unlock() {
	p.lock.Unlock()
}

func (p *Signaler) ListenToSingle(listener *signaler.Listener) {
	p.Lock()
	defer p.Unlock()
	p.listeners.Set(listener.Id, listener)
}

func (p *Signaler) ListenToGroup(listener *signaler.Listener, overrideFunctionaly bool) {
	g, _ := p.RetriveGroup(listener.Id)
	g.Listener = listener
	g.Override = overrideFunctionaly
}

func (p *Signaler) BrdigeGlobally(listener *signaler.GlobalListener, overrideFunctionaly bool) {
	p.LGroupDisabled = true
	p.GlobalBridge = listener
}

func (p *Signaler) ListenToJoin(listener *signaler.JoinListener) {
	p.JListener = listener
}

func (p *Signaler) SignalUser(key string, listenerId string, data any, pack bool) {
	if !strings.Contains(listenerId, "@") {
		return
	}
	username := ""
	p.app.ModifyState(true, func(trx trx.ITrx) error {
		username = string(trx.GetColumn("User", listenerId, "username"))
		return nil
	})
	if username == "" {
		return
	}
	uParts := strings.Split(username, "@")
	if len(uParts) < 2 {
		return
	}
	origin := uParts[1]
	if origin == p.app.Id() {
		listener, found := p.listeners.Get(listenerId)
		if found && (listener != nil) {
			if pack {
				var message string
				switch d := data.(type) {
				case string:
					message = d
				default:
					msg, err := json.Marshal(d)
					if err != nil {
						log.Println(err)
						return
					}
					message = string(msg)
				}
				listener.Signal(key, []byte(message))
			} else {
				listener.Signal(key, data)
			}
		}
	} else {
		p.Federation.SendFedUpdate(origin, key, data, "user", listenerId, []string{})
	}
}

func (p *Signaler) SignalGroup(key string, groupId string, data any, pack bool, exceptions []string) {
	var excepDict = map[string]bool{}
	for _, exc := range exceptions {
		excepDict[exc] = true
	}
	group, ok := p.RetriveGroup(groupId)
	if ok {
		var packet any
		if pack {
			var message []byte
			switch d := data.(type) {
			case string:
				message = []byte(d)
			default:
				msg, err := json.Marshal(d)
				if err != nil {
					log.Println(err)
					return
				}
				message = msg
			}
			packet = message
		} else {
			packet = data
		}
		if p.LGroupDisabled {
			p.GlobalBridge.Signal(groupId, packet)
			return
		}
		if group.Override {
			group.Listener.Signal(key, packet)
			return
		}
		var foreignersMap = map[string][]string{}
		for t := range group.Points.IterBuffered() {
			userId := t.Val
			username := ""
			p.app.ModifyState(true, func(trx trx.ITrx) error {
				username = string(trx.GetColumn("User", userId, "username"))
				return nil
			})
			if username == "" {
				continue
			}
			uParts := strings.Split(username, "@")
			if len(uParts) < 2 {
				continue
			}
			userOrigin := uParts[1]
			log.Println(p.app.Id() + " " + userOrigin)
			if (userOrigin == p.app.Id()) || (userOrigin == "global") {
				if !p.LGroupDisabled || !group.Override {
					if !excepDict[t.Key] {
						listener, found := p.listeners.Get(userId)
						if found && (listener != nil) {
							listener.Signal(key, packet)
						}
					}
				}
			} else {
				if foreignersMap[userOrigin] == nil {
					foreignersMap[userOrigin] = []string{}
				}
				if excepDict[t.Key] {
					foreignersMap[userOrigin] = append(foreignersMap[userOrigin], t.Val)
				}
			}
		}
		for k, v := range foreignersMap {
			p.Federation.SendFedUpdate(k, key, data, "point", groupId, v)
		}
	}
}

func (p *Signaler) JoinGroup(groupId string, userId string) {
	g, ok := p.RetriveGroup(groupId)
	if ok {
		g.Points.Set(userId, userId)
		if p.JListener != nil {
			p.JListener.Join(groupId, userId)
		}
	}
}

func (p *Signaler) LeaveGroup(groupId string, userId string) {
	g, ok := p.RetriveGroup(groupId)
	if ok {
		g.Points.Remove(userId)
		if p.JListener != nil {
			p.JListener.Leave(groupId, userId)
		}
	}
}

func (p *Signaler) RetriveGroup(groupId string) (*signaler.Group, bool) {
	ok := p.groups.Has(groupId)
	if !ok {
		newMap := cmap.New[string]()
		group := &signaler.Group{Points: &newMap, Listener: nil, Override: false}
		p.groups.SetIfAbsent(groupId, group)
	}
	return p.groups.Get(groupId)
}

func NewSignaler(app core.ICore, federation network.IFederation) signaler.ISignaler {
	log.Println("creating signaler...")
	newMap := cmap.New[*signaler.Group]()
	lisMap := cmap.New[*signaler.Listener]()
	return &Signaler{
		app:            app,
		listeners:      &lisMap,
		groups:         &newMap,
		LGroupDisabled: false,
		Federation:     federation,
		GlobalBridge:   nil,
	}
}
