
	package plugger_machine

	import (
		"kasper/src/abstract/models/core"
		"kasper/src/shell/utils"
	    iaction "kasper/src/abstract/models/action"
		actions "kasper/src/shell/api/actions/machine"
	)
	
	type Plugger struct {
		Id      *string
		Actions *actions.Actions
		Core core.ICore
	}
	
		func (c *Plugger) CreateApp() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.CreateApp)
		}
		
		func (c *Plugger) DeleteApp() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.DeleteApp)
		}
		
		func (c *Plugger) UpdateApp() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.UpdateApp)
		}
		
		func (c *Plugger) MyCreatedApps() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.MyCreatedApps)
		}
		
		func (c *Plugger) CreateMachine() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.CreateMachine)
		}
		
		func (c *Plugger) DeleteMachine() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.DeleteMachine)
		}
		
		func (c *Plugger) UpdateMachine() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.UpdateMachine)
		}
		
		func (c *Plugger) Signal() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.Signal)
		}
		
		func (c *Plugger) RunMachine() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.RunMachine)
		}
		
		func (c *Plugger) StopMachine() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.StopMachine)
		}
		
		func (c *Plugger) ReadBuildLogs() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.ReadBuildLogs)
		}
		
		func (c *Plugger) ReadMachineBuilds() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.ReadMachineBuilds)
		}
		
		func (c *Plugger) Deploy() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.Deploy)
		}
		
		func (c *Plugger) ListApps() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.ListApps)
		}
		
		func (c *Plugger) ListMachs() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.ListMachs)
		}
		
		func (c *Plugger) ListAppMachs() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.ListAppMachs)
		}
		
	func (c *Plugger) Install(a *actions.Actions, extra ...any) *Plugger {
		err := actions.Install(a, extra...)
		if err != nil {
			panic(err)
		}
		return c
	}

	func New(actions *actions.Actions, core core.ICore) *Plugger {
		id := "machine"
		return &Plugger{Id: &id, Actions: actions, Core: core}
	}
	