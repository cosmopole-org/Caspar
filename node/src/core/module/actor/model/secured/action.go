package secured

import (
	"encoding/json"
	"errors"
	"kasper/src/abstract/models/action"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/input"
	"kasper/src/abstract/models/update"
	"kasper/src/abstract/state"
	"log"
)

type Parse func(interface{}) (input.IInput, error)

type SecureAction struct {
	action.IAction
	core    core.ICore
	Guard   *Guard
	Parsers map[string]Parse
}

func NewSecureAction(action action.IAction, guard *Guard, core core.ICore, parsers map[string]Parse) *SecureAction {
	return &SecureAction{action, core, guard, parsers}
}

func (a *SecureAction) HasGlobalParser() bool {
	return a.Parsers["*"] != nil
}

func (a *SecureAction) ParseInput(protocol string, raw interface{}) (input.IInput, error) {
	return a.Parsers[protocol](raw)
}

func (a *SecureAction) SecurlyActChain(userId string, packetId string, packetBinary []byte, packetSignature string, input input.IInput, origin string, tag string) {
	success, info := a.Guard.CheckValidityForChain(a.core, packetBinary, packetSignature, userId, input.GetPointId())
	if !success {
		data := []byte("{}")
		a.core.ExecBaseResponseOnChain(packetId, data, a.core.SignPacket(data), 403, "authorization failed", []update.Update{}, tag, userId)
	} else {
		a.core.ModifyStateSecurlyWithSource(false, info, origin, func(s state.IState) error {
			sc, res, err := a.Act(s, input)
			if err != nil {
				data := []byte("{}")
				a.core.ExecBaseResponseOnChain(packetId, data, a.core.SignPacket(data), 500, err.Error(), []update.Update{}, tag, userId)
			} else {
				data, _ := json.Marshal(res)
				a.core.ExecBaseResponseOnChain(packetId, data, a.core.SignPacket(data), sc, "", s.Trx().Updates(), tag, userId)
			}
			return err
		})
	}
}

func (a *SecureAction) SecurelyAct(userId string, packetId string, packetBinary []byte, packetSignature string, input input.IInput, dummy string, insider ...bool) (int, any, error) {
	origin := input.Origin()
	if origin == "" {
		origin = a.core.Id()
	}
	if origin == "global" {
		c := make(chan int, 1)
		var res any
		var sc int
		var e error
		a.core.ExecBaseRequestOnChain(a.Key(), packetBinary, packetSignature, userId, "", func(data []byte, resCode int, err error) {
			result := map[string]any{}
			json.Unmarshal(data, &result)
			res = result
			sc = resCode
			e = err
			c <- 1
		})
		<-c
		return sc, res, e
	}
	if a.core.Id() == origin {
		success, info := a.Guard.CheckValidity(a.core, packetBinary, packetSignature, userId, input.GetPointId(), insider...)
		if !success {
			return -1, nil, errors.New("authorization failed")
		} else {
			var sc int
			var res any
			var err error
			a.core.ModifyStateSecurly(false, info, func(s state.IState) error {
				s.SetSource(origin)
				sc, res, err = a.Act(s, input)
				return err
			})
			return sc, res, err
		}
	}
	success := a.Guard.CheckIdentity(a.core, packetBinary, packetSignature, userId)
	if !success {
		return -1, nil, errors.New("authorization failed")
	}
	cFed := make(chan int, 1)
	var scFed int
	var resFed any
	var errFed error
	a.core.Tools().Network().Federation().SendFedRequestByCallback(origin, packetId, userId, a.Key(), packetBinary, packetSignature, func(data []byte, resCode int, err error) {
		result := map[string]any{}
		e := json.Unmarshal(data, &result)
		if e != nil {
			log.Println(e)
			scFed = 3
			errFed = e
		} else {
			scFed = resCode
			resFed = result
			errFed = err
		}
		cFed <- 1
	})
	<-cFed
	return scFed, resFed, errFed
}

func (a *SecureAction) SecurelyActFed(userId string, packetBinary []byte, packetSignature string, input input.IInput) (int, any, error) {
	success, info := a.Guard.CheckValidity(a.core, packetBinary, packetSignature, userId, input.GetPointId())
	if !success {
		return -1, nil, nil
	}
	var sc int
	var res any
	var err error
	a.core.ModifyStateSecurly(false, info, func(s state.IState) error {
		s.SetSource(input.Origin())
		sc, res, err = a.Act(s, input)
		if res != nil {
			executable, ok := res.(func() (any, error))
			if ok {
				o, e := executable()
				res = o
				err = e
			}
		}
		return err
	})
	return sc, res, err
}
