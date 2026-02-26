package firectl

import (
	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
)

type VM struct {
	Machine  *firecracker.Machine
	Terminal ITerminalManager
	SigCh    chan int
}

type ITerminalManager interface {
	Start()
	Stop()
	SendCommand(string)
}

type IFirectl interface {
	StopVm(id string)
	GetVm(id string) *VM
	RunVm(id string, terminal chan string)
}
