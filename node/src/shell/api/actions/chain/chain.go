package actions_chain

import (
	"encoding/json"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/state"
	inputs_chain "kasper/src/shell/api/inputs/chain"
	"net"
)

type Actions struct {
	App core.ICore
}

func Install(a *Actions, extra ...any) error {
	return nil
}

// Create /chains/create check [ true false false ] access [ true false false false POST ]
func (a *Actions) Create(state state.IState, input inputs_chain.CreateInput) (any, error) {
	id := ""
	if *input.IsTemp {
		id = a.App.Tools().Network().Chain().CreateTempChain()
	} else {
		id = a.App.Tools().Network().Chain().CreateWorkChain()
	}
	return map[string]any{"chainId": id}, nil
}

// SubmitBaseTrx /chains/submitBaseTrx check [ true false false ] access [ true false false false POST ]
func (a *Actions) SubmitBaseTrx(state state.IState, input inputs_chain.SubBaseTrxInput) (any, error) {
	var result []byte
	var resErr error
	a.App.ExecBaseRequestOnChain(input.Key, input.Payload, input.Signature, state.Info().UserId(), "", func(b []byte, i int, err error) {
		result = b
		resErr = err
	})
	if resErr != nil {
		return nil, resErr
	}
	res := map[string]any{}
	e := json.Unmarshal(result, &res)
	if e != nil {
		return nil, e
	}
	return res, nil
}

// RegisterNode /chains/registerNode check [ true false false ] access [ true false false false POST ]
func (a *Actions) RegisterNode(state state.IState, input inputs_chain.RegisterNodeInput) (any, error) {
	ipAddr := ""
	ips, _ := net.LookupIP(input.Orig)
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			ipAddr = ipv4.String()
			break
		}
	}
	state.Trx().PutLink("NodeIpToHost::"+ipAddr, input.Orig)
	return map[string]any{}, nil
}
