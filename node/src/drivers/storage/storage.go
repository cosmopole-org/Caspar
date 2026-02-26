package tool_storage

import (
	"context"
	"encoding/binary"
	"fmt"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/packet"
	"kasper/src/abstract/models/trx"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/google/uuid"

	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type StorageManager struct {
	core        core.ICore
	storageRoot string
	kvdb        *badger.DB
	tsdb        *sql.DB
	lock        sync.Mutex
}

func (sm *StorageManager) StorageRoot() string {
	return sm.storageRoot
}
func (sm *StorageManager) KvDb() *badger.DB {
	return sm.kvdb
}
func (sm *StorageManager) TsDb() *sql.DB {
	return sm.tsdb
}

func (sm *StorageManager) LogBuild(buildId string, machineId string, data string) packet.BuildPacket {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	ctx := context.Background()
	id := uuid.NewString()
	_, err := sm.tsdb.ExecContext(ctx,
		"INSERT INTO buildlogs (id, build_id, machine_id, data) VALUES ($1, $2, $3, $4)",
		id, buildId, machineId, data,
	)
	if err != nil {
		log.Println("Insert error: " + err.Error())
	}
	packet := packet.BuildPacket{Id: id, BuildId: buildId, MachineId: machineId, Data: data}
	return packet
}

func (sm *StorageManager) ReadBuildLogs(buildId string, machineId string) []packet.BuildPacket {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	ctx := context.Background()
	var rows *sql.Rows
	var err error
	rows, err = sm.tsdb.QueryContext(ctx, "SELECT id, data FROM storage WHERE build_id = $1 and machine_id = $2", buildId, machineId)
	if err != nil {
		log.Println(err)
		return []packet.BuildPacket{}
	}
	defer rows.Close()
	logs := []packet.BuildPacket{}
	for rows.Next() {
		var id string
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			log.Println(err)
		}
		logs = append(logs, packet.BuildPacket{Id: id, BuildId: buildId, MachineId: machineId, Data: data})
	}
	return logs
}

func (sm *StorageManager) LogTimeSieries(pointId string, userId string, data string, timeVal int64) packet.LogPacket {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	ctx := context.Background()
	id := uuid.NewString()
	_, err := sm.tsdb.ExecContext(ctx,
		"INSERT INTO storage (id, point_id, user_id, data, time, edited) VALUES ($1, $2, $3, $4, $5, $6)",
		id, pointId, userId, data, timeVal, false,
	)
	if err != nil {
		log.Println("Insert error: " + err.Error())
	}
	packet := packet.LogPacket{Id: id, UserId: userId, Data: data, PointId: pointId, Time: timeVal, Edited: false}
	return packet
}

func (sm *StorageManager) UpdateLog(pointId string, userId string, signalId string, data string, timeVal int64) packet.LogPacket {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	ctx := context.Background()
	_, err := sm.tsdb.ExecContext(ctx,
		"update storage set data = $1 where point_id = $2 and id = $3 and edited = $4",
		data, pointId, signalId, true,
	)
	if err != nil {
		log.Println("Update error: " + err.Error())
	}
	packet := packet.LogPacket{Id: signalId, UserId: userId, Data: data, PointId: pointId, Time: timeVal, Edited: true}
	return packet
}

func (sm *StorageManager) ReadPointLogs(pointId string, beforeTime int64, count int) []packet.LogPacket {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	ctx := context.Background()
	var rows *sql.Rows
	var err error
	if beforeTime == 0 {
		rows, err = sm.tsdb.QueryContext(ctx, "SELECT id, user_id, data, time, edited FROM storage WHERE point_id = $1 order by time desc limit $2", pointId, count)
		if err != nil {
			log.Println(err)
			return []packet.LogPacket{}
		}
		defer rows.Close()
	} else {
		rows, err = sm.tsdb.QueryContext(ctx, "SELECT id, user_id, data, time, edited FROM storage WHERE point_id = $1 and time < $2 order by time desc limit $3", pointId, beforeTime, count)
		if err != nil {
			log.Println(err)
			return []packet.LogPacket{}
		}
		defer rows.Close()
	}
	logs := []packet.LogPacket{}

	for rows.Next() {
		var id string
		var userId string
		var data string
		var timeVal int64
		var edited bool
		if err := rows.Scan(&id, &userId, &data, &timeVal, &edited); err != nil {
			log.Println(err)
		}
		logs = append(logs, packet.LogPacket{Id: id, UserId: userId, Data: data, PointId: pointId, Time: timeVal, Edited: edited})
	}
	return logs
}

func (sm *StorageManager) PickPointLogs(pointId string, ids []string) []packet.LogPacket {

	ctx := context.Background()
	if len(ids) == 0 {
		return []packet.LogPacket{}
	}
	rows, err := sm.tsdb.QueryContext(ctx, "SELECT id, user_id, data, time, edited FROM storage WHERE point_id = $1 and id in ('"+strings.Join(ids, "','")+"')", pointId)
	if err != nil {
		log.Println(err)
		return []packet.LogPacket{}
	}
	defer rows.Close()
	logs := []packet.LogPacket{}
	fmt.Println("Query results:")
	for rows.Next() {
		var id string
		var userId string
		var data string
		var timeVal int64
		var edited bool
		if err := rows.Scan(&id, &userId, &data, &timeVal, &edited); err != nil {
			log.Println(err)
		}
		logs = append(logs, packet.LogPacket{Id: id, UserId: userId, Data: data, PointId: pointId, Time: timeVal, Edited: edited})
	}
	return logs
}

func (sm *StorageManager) GenId(t trx.ITrx, origin string) string {
	sm.lock.Lock()
	defer sm.lock.Unlock()
	if origin == "global" {
		item := t.GetBytes("globalIdCounter")
		var counter int64 = 0
		if len(item) == 0 {
			counter = 0
		} else {
			counter = int64(binary.BigEndian.Uint64(item))
		}
		counter++
		nextB := [8]byte{}
		binary.BigEndian.PutUint64(nextB[:], uint64(counter))
		t.PutBytes("globalIdCounter", nextB[:])
		return fmt.Sprintf("%d@%s", counter, origin)
	} else {
		trx := sm.kvdb.NewTransaction(true)
		defer trx.Commit()
		item, err := trx.Get([]byte("localIdCounter"))
		var counter int64 = 0
		if err != nil {
			counter = 0
		} else {
			var b []byte
			item.Value(func(val []byte) error {
				b = val
				return nil
			})
			counter = int64(binary.BigEndian.Uint64(b))
		}
		counter++
		nextB := [8]byte{}
		binary.BigEndian.PutUint64(nextB[:], uint64(counter))
		trx.Set([]byte("localIdCounter"), nextB[:])
		return fmt.Sprintf("%d@%s", counter, origin)
	}
}

func NewStorage(core core.ICore, storageRoot string, baseDbPath string, logsDbPath string, searcherDbPath string) *StorageManager {
	log.Println("connecting to database...")
	os.MkdirAll(baseDbPath, os.ModePerm)
	kvdb, err := badger.Open(badger.DefaultOptions(baseDbPath).WithSyncWrites(true))
	if err != nil {
		panic(err)
	}
	tsdb, err := sql.Open("pgx", "postgres://admin:quest@localhost:8812/qdb?sslmode=disable")
	if err != nil {
		panic(err)
	}
	for {
		_, err = tsdb.ExecContext(context.Background(),
			"create table if not exists storage(id text, point_id text, user_id text, data text, time bigint, edited boolean);",
		)
		if err != nil {
			log.Println(err)
			time.Sleep(2 * time.Second)
		} else {
			break
		}
	}
	_, err = tsdb.ExecContext(context.Background(),
		"create table if not exists buildlogs(id text, build_id text, machine_id text, data text);",
	)
	if err != nil {
		panic(err)
	}
	return &StorageManager{core: core, tsdb: tsdb, kvdb: kvdb, storageRoot: storageRoot}
}
