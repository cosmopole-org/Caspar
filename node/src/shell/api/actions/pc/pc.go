package actions_pc

import (
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/packet"
	"kasper/src/abstract/state"
	inputs_pc "kasper/src/shell/api/inputs/pc"
	"kasper/src/shell/utils/crypto"
	"kasper/src/shell/utils/future"
)

type Actions struct {
	App core.ICore
}

func Install(a *Actions, extra ...any) error {
	return nil
}

// RunPc /pc/runPc check [ true false false ] access [ true false false false POST ]
func (a *Actions) RunPc(state state.IState, input inputs_pc.RunPcInput) (any, error) {
	vmId := crypto.SecureUniqueId(a.App.Id())
	terminal := make(chan string)
	future.Async(func() {
		for {
			message := <-terminal
			if message == "" {
				return
			}
			a.App.Tools().Signaler().SignalUser("pc/message", state.Info().UserId(), packet.ResponseSimpleMessage{Message: message}, true)
		}
	}, false)
	a.App.Tools().Firectl().RunVm(vmId, terminal)
	return map[string]any{"vmId": vmId}, nil
}

// ExecCommand /pc/execCommand check [ true false false ] access [ true false false false POST ]
func (a *Actions) ExecCommand(state state.IState, input inputs_pc.ExecCommandInput) (any, error) {
	a.App.Tools().Firectl().GetVm(input.VmId).Terminal.SendCommand(input.Command)
	return map[string]any{}, nil
}
