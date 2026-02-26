package elpis

/*
 #cgo CXXFLAGS: -std=c++17
 #cgo LDFLAGS: -lrocksdb -lpthread -lz -lsnappy -lzstd -llz4 -lbz2 -lwasmedge -static-libgcc -static-libstdc++

 #include <stdlib.h>
 #include "main.h"
*/
import "C"

import (
	"encoding/json"
	"errors"
	"kasper/src/abstract/adapters/signaler"
	"kasper/src/abstract/adapters/storage"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/worker"
	inputs_points "kasper/src/shell/api/inputs/points"
	"kasper/src/shell/utils/future"
	"log"
)

type Elpis struct {
	app         core.ICore
	storageRoot string
	storage     storage.IStorage
}

func (wm *Elpis) Assign(machineId string) {
	wm.app.Tools().Signaler().ListenToSingle(&signaler.Listener{
		Id: machineId,
		Signal: func(key string, a any) {
			astPath := C.CString(wm.app.Tools().Storage().StorageRoot() + "/machines/" + machineId + "/module")
			data := string(a.([]byte))
			if key == "points/signal" {
				var inp inputs_points.SignalInput
				e := json.Unmarshal([]byte(data), &inp)
				if e != nil {
					log.Println(e)
				}
				pointId := C.CString(inp.PointId)
				userId := C.CString(inp.UserId)
				sendType := C.CString(inp.Type)
				inputData := C.CString(inp.Data)
				future.Async(func() {
					C.runVm(astPath, sendType, pointId, userId, inputData)
				}, false)
			}
		},
	})
}

func (wm *Elpis) ExecuteChainTrxsGroup([]*worker.Trx) {

}

func (wm *Elpis) ElpisCallback(dataRaw string) string {
	data := map[string]any{}
	err := json.Unmarshal([]byte(dataRaw), &data)
	if err != nil {
		log.Println(err)
		return err.Error()
	}
	key, err := checkField[string](data, "key", "")
	if err != nil {
		log.Println(err)
		return err.Error()
	}
	input, err := checkField[map[string]any](data, "input", nil)
	if err != nil {
		log.Println(err)
		return err.Error()
	}
	if key == "log" {
		text, err := checkField(input, "text", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		log.Println("elpis vm:", text)
	}
	return "{}"
}

func checkField[T any](input map[string]any, fieldName string, defVal T) (T, error) {
	fRaw, ok := input[fieldName]
	if !ok {
		return defVal, errors.New("{\"error\":1}}")
	}
	f, ok := fRaw.(T)
	if !ok {
		return defVal, errors.New("{\"error\":2}}")
	}
	return f, nil
}

func NewElpis(core core.ICore, storageRoot string, storage storage.IStorage) *Elpis {
	wm := &Elpis{
		app:         core,
		storageRoot: storageRoot,
		storage:     storage,
	}
	return wm
}
