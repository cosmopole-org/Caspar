package actions_auth

import (
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/state"
	inputsauth "kasper/src/shell/api/inputs/auth"
	outputsauth "kasper/src/shell/api/outputs/auth"
)

type Actions struct {
	App core.ICore
}

func Install(a *Actions, extra ...any) error {
	return nil
}

// GetServerPublicKey /auths/getServerPublicKey check [ false false false ] access [ true false false false GET ]
func (a *Actions) GetServerPublicKey(_ state.IState, _ inputsauth.GetServerKeyInput) (any, error) {
	return &outputsauth.GetServerKeyOutput{PublicKey: string(a.App.Tools().Security().FetchKeyPair("server_key")[1])}, nil
}

// GetServersMap /auths/getServersMap check [ false false false ] access [ true false false false GET ]
func (a *Actions) GetServersMap(_ state.IState, _ inputsauth.GetServersMapInput) (any, error) {
	return outputsauth.GetServersMapOutput{Servers: a.App.Tools().Network().Chain().Peers()}, nil
}
