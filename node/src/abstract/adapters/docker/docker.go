package docker

import (
	models "kasper/src/shell/api/model"
)

type IDocker interface {
	Assign(machineId string)
	SaRContainer(machineId string, imageName string, containerName string)
	RunContainer(machineId string, pointId string, imageName string, containerName string, inputFile map[string]string, standalone bool) (*models.File, error)
	BuildImage(dockerfile string, machineId string, imageName string, outputChan chan string) error
	ExecContainer(machineId string, imageName string, containerName string, command string) (string, error)
	CopyToContainer(machineId string, imageName string, containerName string, fileName string, content string) error
	RunGateway()
}
