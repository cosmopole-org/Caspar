package elpis

import "kasper/src/abstract/models/worker"

type IElpis interface {
	Assign(machineId string)
	ExecuteChainTrxsGroup([]*worker.Trx)
	ElpisCallback(dataRaw string) string
}
