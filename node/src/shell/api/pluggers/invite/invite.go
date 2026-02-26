
	package plugger_invite

	import (
		"kasper/src/abstract/models/core"
		"kasper/src/shell/utils"
	    iaction "kasper/src/abstract/models/action"
		actions "kasper/src/shell/api/actions/invite"
	)
	
	type Plugger struct {
		Id      *string
		Actions *actions.Actions
		Core core.ICore
	}
	
		func (c *Plugger) Create() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.Create)
		}
		
		func (c *Plugger) ListPointInvites() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.ListPointInvites)
		}
		
		func (c *Plugger) ListUserInvites() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.ListUserInvites)
		}
		
		func (c *Plugger) Cancel() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.Cancel)
		}
		
		func (c *Plugger) Accept() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.Accept)
		}
		
		func (c *Plugger) Decline() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.Decline)
		}
		
	func (c *Plugger) Install(a *actions.Actions, extra ...any) *Plugger {
		err := actions.Install(a, extra...)
		if err != nil {
			panic(err)
		}
		return c
	}

	func New(actions *actions.Actions, core core.ICore) *Plugger {
		id := "invite"
		return &Plugger{Id: &id, Actions: actions, Core: core}
	}
	