
	package plugger_chain

	import (
		"kasper/src/abstract/models/core"
		"kasper/src/shell/utils"
	    iaction "kasper/src/abstract/models/action"
		actions "kasper/src/shell/api/actions/chain"
	)
	
	type Plugger struct {
		Id      *string
		Actions *actions.Actions
		Core core.ICore
	}
	
		func (c *Plugger) Create() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.Create)
		}
		
		func (c *Plugger) SubmitBaseTrx() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.SubmitBaseTrx)
		}
		
		func (c *Plugger) RegisterNode() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.RegisterNode)
		}
		
	func (c *Plugger) Install(a *actions.Actions, extra ...any) *Plugger {
		err := actions.Install(a, extra...)
		if err != nil {
			panic(err)
		}
		return c
	}

	func New(actions *actions.Actions, core core.ICore) *Plugger {
		id := "chain"
		return &Plugger{Id: &id, Actions: actions, Core: core}
	}
	