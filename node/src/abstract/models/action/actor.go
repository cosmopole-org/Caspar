package action

type IActor interface {
	InjectAction(IAction)
	InjectService(interface{})
	FetchAction(string) IAction
}
