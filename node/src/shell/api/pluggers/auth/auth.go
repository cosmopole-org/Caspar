
	package plugger_auth

	import (
		"kasper/src/abstract/models/core"
		"kasper/src/shell/utils"
	    iaction "kasper/src/abstract/models/action"
		actions "kasper/src/shell/api/actions/auth"
	)
	
	type Plugger struct {
		Id      *string
		Actions *actions.Actions
		Core core.ICore
	}
	
		func (c *Plugger) GetServerPublicKey() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.GetServerPublicKey)
		}
		
		func (c *Plugger) GetServersMap() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.GetServersMap)
		}
		
	func (c *Plugger) Install(a *actions.Actions, extra ...any) *Plugger {
		err := actions.Install(a, extra...)
		if err != nil {
			panic(err)
		}
		return c
	}

	func New(actions *actions.Actions, core core.ICore) *Plugger {
		id := "auth"
		return &Plugger{Id: &id, Actions: actions, Core: core}
	}
	