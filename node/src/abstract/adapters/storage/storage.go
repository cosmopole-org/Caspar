package storage

import (
	"kasper/src/abstract/models/packet"
	"kasper/src/abstract/models/trx"

	"database/sql"
	"github.com/dgraph-io/badger"
)

type IStorage interface {
	StorageRoot() string
	KvDb() *badger.DB
	TsDb() *sql.DB
	GenId(t trx.ITrx, origin string) string
	LogTimeSieries(pointId string, userId string, data string, timeVal int64) packet.LogPacket
	UpdateLog(pointId string, userId string, signalId string, data string, timeVal int64) packet.LogPacket
	ReadPointLogs(pointId string, beforeTime int64, count int) []packet.LogPacket
	PickPointLogs(pointId string, ids []string) []packet.LogPacket
	LogBuild(buildId string, machineId string, data string) packet.BuildPacket
	ReadBuildLogs(buildId string, machineId string) []packet.BuildPacket
}
