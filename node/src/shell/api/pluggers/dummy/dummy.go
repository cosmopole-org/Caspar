
	package plugger_dummy

	import (
		"kasper/src/abstract/models/core"
		"kasper/src/shell/utils"
	    iaction "kasper/src/abstract/models/action"
		actions "kasper/src/shell/api/actions/dummy"
	)
	
	type Plugger struct {
		Id      *string
		Actions *actions.Actions
		Core core.ICore
	}
	
		func (c *Plugger) Hello() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.Hello)
		}
		
		func (c *Plugger) Time() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.Time)
		}
		
		func (c *Plugger) Ping() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.Ping)
		}
		
	func (c *Plugger) Install(a *actions.Actions, extra ...any) *Plugger {
		err := actions.Install(a, extra...)
		if err != nil {
			panic(err)
		}
		return c
	}

	func New(actions *actions.Actions, core core.ICore) *Plugger {
		id := "dummy"
		return &Plugger{Id: &id, Actions: actions, Core: core}
	}
	