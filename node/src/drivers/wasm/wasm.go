package wasm

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"kasper/src/abstract/adapters/docker"
	"kasper/src/abstract/adapters/file"
	"kasper/src/abstract/adapters/signaler"
	"kasper/src/abstract/adapters/storage"
	iaction "kasper/src/abstract/models/action"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/packet"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/models/update"
	"kasper/src/abstract/models/worker"
	"kasper/src/abstract/state"
	"kasper/src/core/module/actor/model/base"
	inputs_points "kasper/src/shell/api/inputs/points"
	inputs_storage "kasper/src/shell/api/inputs/storage"
	inputs_users "kasper/src/shell/api/inputs/users"
	"kasper/src/shell/api/model"
	updates_points "kasper/src/shell/api/updates/points"
	"kasper/src/shell/utils/future"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	zmq "github.com/pebbe/zmq4"
)

type Wasm struct {
	app         core.ICore
	storageRoot string
	storage     storage.IStorage
	docker      docker.IDocker
	file        file.IFile
	aeSocket    chan string
}

func (wm *Wasm) Assign(machineId string) {
	wm.app.Tools().Signaler().ListenToSingle(&signaler.Listener{
		Id: machineId,
		Signal: func(key string, a any) {
			astPath := wm.app.Tools().Storage().StorageRoot() + "/machines/" + machineId + "/module"
			data := string(a.([]byte))
			if key == "points/signal" {
				str, _ := json.Marshal(map[string]any{
					"type":      "runOffChain",
					"machineId": machineId,
					"input":     data,
					"astPath":   astPath,
				})
				wm.aeSocket <- string(str)
			}
		},
	})
}

func (wm *Wasm) ExecuteChainTrxsGroup(trxs []*worker.Trx) {
	b, e := json.Marshal(trxs)
	if e != nil {
		println(e)
		return
	}
	input := string(b)
	astStorePath := wm.app.Tools().Storage().StorageRoot() + "/machines"
	str, _ := json.Marshal(map[string]any{
		"type":         "runOnChain",
		"astStorePath": astStorePath,
		"input":        input,
	})
	wm.aeSocket <- string(str)
}

func (wm *Wasm) ExecuteChainEffects(effects string) {
	str, _ := json.Marshal(map[string]any{
		"type":    "applyTrxEffects",
		"effects": effects,
	})
	wm.aeSocket <- string(str)
}

type ChainDbOp struct {
	OpType string `json:"opType"`
	Key    string `json:"key"`
	Val    string `json:"val"`
}

func (wm *Wasm) RunVm(machineId string, pointId string, data string) {
	point := model.Point{Id: pointId}
	isMemberOfPoint := false
	wm.app.ModifyState(true, func(trx trx.ITrx) error {
		point.Pull(trx)
		isMemberOfPoint = (trx.GetLink("memberof::"+machineId+"::"+pointId) == "true")
		return nil
	})
	if !isMemberOfPoint {
		return
	}
	astPath := wm.app.Tools().Storage().StorageRoot() + "/machines/" + machineId + "/module"
	b, _ := json.Marshal(updates_points.Send{User: model.User{}, Point: point, Action: "single", Data: data})
	input := string(b)
	str, _ := json.Marshal(map[string]any{
		"type":      "runOffChain",
		"machineId": machineId,
		"input":     input,
		"astPath":   astPath,
	})
	wm.aeSocket <- string(str)
}

func (wm *Wasm) WasmCallback(dataRaw string) (string, int64) {
	println(dataRaw)
	data := map[string]any{}
	err := json.Unmarshal([]byte(dataRaw), &data)
	if err != nil {
		println(err)
		return err.Error(), 0
	}
	reqIdRaw, err := checkField(data, "requestId", float64(0))
	if err != nil {
		println(err)
		return err.Error(), 0
	}
	reqId := int64(reqIdRaw)
	key, err := checkField(data, "key", "")
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	input, err := checkField[map[string]any](data, "input", nil)
	if err != nil {
		println(err)
		return err.Error(), reqId
	}
	if key == "runDocker" {
		machineId, err := checkField(input, "machineId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		pointId, err := checkField(input, "pointId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		found := false
		wm.app.ModifyState(true, func(trx trx.ITrx) error {
			if trx.GetLink("member::"+pointId+"::"+machineId) == "true" {
				found = true
			}
			return nil
		})
		if !found {
			err := errors.New("access denied")
			println(err)
			return err.Error(), reqId
		}
		inputFilesStr, err := checkField(input, "inputFiles", "{}")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		inputFiles := map[string]string{}
		err = json.Unmarshal([]byte(inputFilesStr), &inputFiles)
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		finalInputFiles := map[string]string{}
		for k, v := range inputFiles {
			if !wm.file.CheckFileFromStorage(wm.storageRoot, pointId, k) {
				err := errors.New("input file does not exist")
				println(err)
				return err.Error(), reqId
			}
			path := fmt.Sprintf("%s/files/%s/%s", wm.storageRoot, pointId, k)
			finalInputFiles[path] = v
		}
		imageName, err := checkField(input, "imageName", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		cn, err := checkField(input, "containerName", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		cnParts := strings.Split(cn, "|")
		containerName := cnParts[0]
		isAsync := cnParts[1] == "true"
		creatorUserId := cnParts[2]
		creatorSignature := cnParts[3]
		lockId := cnParts[4]
		inp, _ := json.Marshal(inputs_users.ConsumeLockInput{
			Type:      "pay",
			UserId:    creatorUserId,
			Signature: creatorSignature,
			LockId:    lockId,
			Amount:    100,
		})
		sign := wm.app.SignPacketAsOwner(inp)
		resChan := make(chan bool)
		wm.app.ExecBaseRequestOnChain("/users/consumeLock", inp, sign, wm.app.OwnerId(), "", func(b []byte, i int, err error) {
			if err != nil {
				println(err)
				resChan <- false
			} else {
				resChan <- true
			}
		})
		if res := <-resChan; res {
			if isAsync {
				future.Async(func() {
					if imageName != "main" || containerName != "main" {
						wm.docker.Assign(machineId + "_" + imageName + "_" + containerName)
					}
					wm.docker.SaRContainer(machineId, imageName, containerName)
					wm.docker.RunContainer(machineId, pointId, imageName, containerName, finalInputFiles, false)
				}, false)
			} else {
				wm.docker.SaRContainer(machineId, imageName, containerName)
				outputFile, err := wm.docker.RunContainer(machineId, pointId, imageName, containerName, finalInputFiles, false)
				if err != nil {
					println(err)
					return err.Error(), reqId
				}
				if outputFile != nil {
					str, err := json.Marshal(outputFile)
					if err != nil {
						println(err)
						return err.Error(), reqId
					}
					return string(str), reqId
				}
			}
		}
	} else if key == "execDocker" {
		machineId, err := checkField(input, "machineId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		imageName, err := checkField(input, "imageName", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		containerName, err := checkField(input, "containerName", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		command, err := checkField(input, "command", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		output, err := wm.docker.ExecContainer(machineId, imageName, containerName, command)
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		return output, reqId
	} else if key == "copyToDocker" {
		machineId, err := checkField(input, "machineId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		imageName, err := checkField(input, "imageName", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		containerName, err := checkField(input, "containerName", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		fileName, err := checkField(input, "fileName", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		content, err := checkField(input, "content", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		err = wm.docker.CopyToContainer(machineId, imageName, containerName, fileName, content)
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		return "", reqId
	} else if key == "log" {
		_, err := checkField(input, "text", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		// println("elpis vm:", text)
	} else if key == "httpPost" {
		url, err := checkField(input, "url", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		method := strings.Split(url, "|")[0]
		url = url[len(method)+1:]
		headers, err := checkField(input, "headers", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		body, err := checkField(input, "body", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		req, err := http.NewRequest(method, url, bytes.NewBuffer([]byte(body)))
		if err != nil {
			println("Error creating request:" + err.Error())
			return err.Error(), reqId
		}
		heads := map[string]string{}
		err = json.Unmarshal([]byte(headers), &heads)
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		for k, v := range heads {
			req.Header.Set(k, v)
		}
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			println("Request failed:" + err.Error())
			return err.Error(), reqId
		}
		defer resp.Body.Close()
		println("Response status:" + resp.Status)
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			println("Error reading response body:" + err.Error())
			return err.Error(), reqId
		}
		return base64.StdEncoding.EncodeToString(bodyBytes), reqId
	} else if key == "checkTokenValidity" {
		tokenOwnerId, err := checkField(input, "tokenOwnerId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		tokenId, err := checkField(input, "tokenId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		gasLimit := int64(0)
		wm.app.ModifyState(true, func(trx trx.ITrx) error {
			if trx.GetString("Temp::User::"+tokenOwnerId+"::consumedTokens::"+tokenId) == "true" {
				return nil
			}
			if m, e := trx.GetJson("Json::User::"+tokenOwnerId, "lockedTokens."+tokenId); e == nil {
				gasLimit = int64(m["amount"].(float64))
			}
			return nil
		})
		jsn, _ := json.Marshal(map[string]any{"gasLimit": gasLimit})
		return string(jsn), reqId
	} else if key == "submitOnchainResponse" {
		callbackId, err := checkField(input, "callbackId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		cost, err := checkField[float64](input, "cost", 0)
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		tokenOwnerId, err := checkField(input, "tokenOwnerId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		tokenId, err := checkField(input, "tokenId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		pack, err := checkField(input, "packet", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		changes, err := checkField(input, "changes", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		resCode, err := checkField[float64](input, "resCode", 0)
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		e, err := checkField(input, "error", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		trxInp := packet.ConsumeTokenInput{TokenId: tokenId, Amount: int64(cost), TokenOwnerId: tokenOwnerId}
		i, _ := json.Marshal(trxInp)
		wm.app.ModifyState(false, func(trx trx.ITrx) error {
			trx.PutString("Temp::User::"+tokenOwnerId+"::consumedTokens::"+tokenId, "true")
			return nil
		})
		wm.app.ExecAppletResponseOnChain(callbackId, []byte(pack), "#appletsign", int(resCode), e, []update.Update{{Val: []byte("consumeToken: " + string(i))}, {Val: []byte("applet: " + changes)}})
	} else if key == "submitOnchainTrx" {
		machineId, err := checkField(input, "machineId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		targetMachineId, err := checkField(input, "targetMachineId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		isRequesterOnchain, err := checkField(input, "isRequesterOnchain", false)
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		kRaw, err := checkField(input, "key", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		kParts := strings.Split(kRaw, "|")
		dstPointId := kParts[0]
		srcPointId, err := checkField(input, "pointId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		k := kParts[1]
		userId := kParts[2]
		userSignature := kParts[3]
		tokenId := kParts[4]
		onchainReq := kParts[5] == "true"
		isFile, err := checkField(input, "isFile", false)
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		isBase, err := checkField(input, "isBase", false)
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		tag, err := checkField(input, "tag", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		pack, err := checkField(input, "packet", "{}")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		var data []byte
		if isFile {
			if wm.file.CheckFileFromStorage(wm.storageRoot, srcPointId, pack) {
				b, err := wm.file.ReadFileFromStorage(wm.storageRoot, srcPointId, pack)
				if err != nil {
					println(err)
					return err.Error(), reqId
				}
				data = b
			}
		} else {
			data = []byte(pack)
		}

		if userId == "" && userSignature == "" {
			userId = machineId
			userSignature = "#appletsign"
		}

		result := []byte("{}")
		outputCnan := make(chan int)
		if isBase {
			if k == "/storage/upload" {
				inp := inputs_storage.UploadDataInput{
					Data:    base64.StdEncoding.EncodeToString(data),
					PointId: dstPointId,
				}
				data, _ = json.Marshal(inp)
			}
			if onchainReq {
				wm.app.ExecBaseRequestOnChain(k, data, userSignature, userId, tag, func(b []byte, i int, err error) {
					if err != nil {
						println(err)
						return
					}
					result = b
					if !isRequesterOnchain {
						outputCnan <- 1
					}
				})
			} else {
				action := wm.app.Actor().FetchAction(k)
				if action == nil {
					return "action not found", reqId
				}
				var err error
				inp, err := action.(iaction.ISecureAction).ParseInput("tcp", data)
				if err != nil {
					println(err)
					return err.Error(), reqId
				}
				_, result, err := action.(iaction.ISecureAction).SecurelyAct(userId, "", data, userSignature, inp, "", true)
				println(result)
				if err != nil {
					return err.Error(), reqId
				}
				str, _ := json.Marshal(result)
				return string(str), reqId
			}
		} else {
			if onchainReq {
				wm.app.ExecAppletRequestOnChain(dstPointId, targetMachineId, k, data, userSignature, userId, tag, tokenId, func(b []byte, i int, err error) {
					if err != nil {
						println(err)
						return
					}
					result = b
					if !isRequesterOnchain {
						outputCnan <- 1
					}
				})
			}
		}
		if !isRequesterOnchain {
			<-outputCnan
		}
		if !isRequesterOnchain {
			return string(result), reqId
		} else {
			return "{}", reqId
		}
	} else if key == "plantTrigger" {
		count, err := checkField(input, "count", float64(0))
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		machineId, err := checkField(input, "machineId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		tag, err := checkField(input, "tag", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		pointId, err := checkField(input, "pointId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		data, err := checkField(input, "input", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		if tag == "alarm" {
			future.Async(func() {
				wm.app.ModifyState(false, func(trx trx.ITrx) error {
					trx.PutLink("vmAlarmPointId::"+machineId, pointId)
					trx.PutLink("vmAlarmData::"+machineId, data)
					trx.PutLink("vmAlarmTime::"+machineId, fmt.Sprintf("%d", time.Now().UnixMilli()+(int64(count)*1000)))
					return nil
				})
				time.Sleep(time.Duration(count) * time.Second)
				wm.app.ModifyState(false, func(trx trx.ITrx) error {
					trx.DelKey("link::vmAlarmPointId::" + machineId)
					trx.DelKey("link::vmAlarmData::" + machineId)
					trx.DelKey("link::vmAlarmTime::" + machineId)
					return nil
				})
				if wm.app.Tools().Security().HasAccessToPoint(machineId, pointId) {
					wm.RunVm(machineId, pointId, data)
				}
			}, false)
		} else {
			wm.app.PlantChainTrigger(int(count), machineId, tag, machineId, pointId, data)
		}
	} else if key == "signalPoint" {
		machineId, err := checkField(input, "machineId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		typAndTemp, err := checkField(input, "type", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		parts := strings.Split(typAndTemp, "|")
		typ := parts[0]
		temp := false
		if len(parts) > 1 {
			if parts[1] == "true" {
				temp = true
			} else if parts[1] == "false" {
				temp = false
			}
		}
		pointId, err := checkField(input, "pointId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		userId, err := checkField(input, "userId", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		data, err := checkField(input, "data", "")
		if err != nil {
			println(err)
			return err.Error(), reqId
		}
		wm.app.ModifyStateSecurly(false, base.NewInfo(machineId, pointId), func(s state.IState) error {
			_, _, err := wm.app.Actor().FetchAction("/points/signal").Act(s, inputs_points.SignalInput{
				Type:    typ,
				Data:    data,
				PointId: pointId,
				UserId:  userId,
				Temp:    temp,
			})
			return err
		})
	}

	return "{}", reqId
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
	if newF, ok := fRaw.(string); ok {
		return any(string([]byte(newF))).(T), nil
	}
	return f, nil
}

func NewWasm(core core.ICore, storageRoot string, storage storage.IStorage, kvDbPath string, docker docker.IDocker, file file.IFile) *Wasm {
	os.MkdirAll(kvDbPath, os.ModePerm)
	wm := &Wasm{
		app:         core,
		storageRoot: storageRoot,
		storage:     storage,
		docker:      docker,
		file:        file,
		aeSocket:    make(chan string, 1000),
	}
	future.Async(func() {
		zctx, _ := zmq.NewContext()
		s, _ := zctx.NewSocket(zmq.REP)
		s.Bind("tcp://*:5555")

		zctx2, _ := zmq.NewContext()
		fmt.Printf("Connecting to the app engine server...\n")
		s2, _ := zctx2.NewSocket(zmq.REQ)
		s2.Connect("tcp://localhost:5556")

		future.Async(func() {
			for {
				msg := <-wm.aeSocket
				s2.Send(msg, 0)
				s2.Recv(0)
			}
		}, true)

		for {
			msg, _ := s.Recv(0)
			log.Printf("Received %s\n", msg)
			future.Async(func() {
				res, reqId := wm.WasmCallback(msg)
				result, _ := json.Marshal(map[string]any{
					"type":      "apiResponse",
					"requestId": reqId,
					"data":      res,
				})
				wm.aeSocket <- string(result)
			}, false)
			s.Send("", 0)
		}
	}, true)
	return wm
}

func (wm *Wasm) CloseKVDB() {
	// C.close()
}
