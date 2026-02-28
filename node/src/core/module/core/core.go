package module_core

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"kasper/src/abstract/adapters/docker"
	"kasper/src/abstract/adapters/elpis"
	"kasper/src/abstract/adapters/file"
	"kasper/src/abstract/adapters/firectl"
	"kasper/src/abstract/adapters/network"
	"kasper/src/abstract/adapters/security"
	"kasper/src/abstract/adapters/signaler"
	"kasper/src/abstract/adapters/storage"
	"kasper/src/abstract/adapters/tools"
	"kasper/src/abstract/adapters/wasm"
	iaction "kasper/src/abstract/models/action"
	"kasper/src/abstract/models/chain"
	"kasper/src/abstract/models/info"
	"kasper/src/abstract/models/input"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/models/update"
	"kasper/src/abstract/models/worker"
	"kasper/src/abstract/state"
	actor "kasper/src/core/module/actor"
	mainstate "kasper/src/core/module/actor/model/state"
	module_trx "kasper/src/core/module/actor/model/trx"
	mach_model "kasper/src/shell/api/model"
	"kasper/src/shell/utils/crypto"
	"kasper/src/shell/utils/future"

	driver_docker "kasper/src/drivers/docker"
	driver_elpis "kasper/src/drivers/elpis"
	driver_file "kasper/src/drivers/file"
	driver_firectl "kasper/src/drivers/firectl"
	driver_network "kasper/src/drivers/network"
	driver_security "kasper/src/drivers/security"
	driver_signaler "kasper/src/drivers/signaler"
	driver_storage "kasper/src/drivers/storage"
	driver_wasm "kasper/src/drivers/wasm"

	driver_network_fed "kasper/src/drivers/network/federation"

	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	cryp "crypto"
	"crypto/rand"
)

type Tools struct {
	security security.ISecurity
	signaler signaler.ISignaler
	storage  storage.IStorage
	network  network.INetwork
	file     file.IFile
	wasm     wasm.IWasm
	elpis    elpis.IElpis
	docker   docker.IDocker
	firectl  firectl.IFirectl
}

func (t *Tools) Security() security.ISecurity {
	return t.security
}

func (t *Tools) Signaler() signaler.ISignaler {
	return t.signaler
}

func (t *Tools) Storage() storage.IStorage {
	return t.storage
}

func (t *Tools) Network() network.INetwork {
	return t.network
}

func (t *Tools) File() file.IFile {
	return t.file
}

func (t *Tools) Wasm() wasm.IWasm {
	return t.wasm
}

func (t *Tools) Elpis() elpis.IElpis {
	return t.elpis
}

func (t *Tools) Docker() docker.IDocker {
	return t.docker
}

func (t *Tools) Firectl() firectl.IFirectl {
	return t.firectl
}

type Core struct {
	lock             sync.Mutex
	triggerLock      sync.Mutex
	ownerId          string
	ownerPrivKey     *rsa.PrivateKey
	id               string
	tools            tools.ITools
	started          bool
	gods             []string
	chain            chan any
	chainCallbacks   map[string]*chain.ChainCallback
	Ip               string
	elections        []chain.Election
	elecReg          bool
	elecStarter      string
	elecStartTime    int64
	executors        map[string]bool
	appPendingTrxs   []*worker.Trx
	actionStore      iaction.IActor
	privKey          *rsa.PrivateKey
	messageCallbacks map[string]*chain.MessageCallback
}

var MAX_VALIDATOR_COUNT = 5

func NewCore(origin string, ownerId string, ownerPrivateKey *rsa.PrivateKey) *Core {
	id := origin
	execs := map[string]bool{}
	execs[os.Getenv("ROOT_NODE")] = true
	return &Core{
		ownerId:          ownerId,
		ownerPrivKey:     ownerPrivateKey,
		id:               id,
		gods:             make([]string, 0),
		chain:            nil,
		chainCallbacks:   map[string]*chain.ChainCallback{},
		messageCallbacks: map[string]*chain.MessageCallback{},
		Ip:               id,
		elections:        nil,
		elecReg:          false,
		executors:        execs,
		actionStore:      actor.NewActor(),
		started:          false,
	}
}

func (c *Core) Executors() map[string]bool {
	return c.executors
}

func (c *Core) SetExecutors(execs map[string]bool) {
	c.executors = execs
}

func (c *Core) Actor() iaction.IActor {
	return c.actionStore
}

func (c *Core) ModifyStateSecurlyWithSource(readonly bool, info info.IInfo, src string, fn func(state.IState) error) {
	trx := module_trx.NewTrx(c, c.Tools().Storage(), readonly)
	var err error
	defer func() {
		if err == nil {
			trx.Commit()
		} else {
			trx.Discard()
		}
	}()
	s := mainstate.NewState(info, trx, src)
	err = fn(s)
}

func (c *Core) ModifyStateSecurly(readonly bool, info info.IInfo, fn func(state.IState) error) {
	trx := module_trx.NewTrx(c, c.Tools().Storage(), readonly)
	var err error
	defer func() {
		if err == nil {
			trx.Commit()
		} else {
			trx.Discard()
		}
	}()
	s := mainstate.NewState(info, trx)
	err = fn(s)
}

func (c *Core) ModifyState(readonly bool, fn func(trx.ITrx) error) {
	trx := module_trx.NewTrx(c, c.Tools().Storage(), readonly)
	var err error
	defer func() {
		if err == nil {
			trx.Commit()
		} else {
			trx.Discard()
		}
	}()
	err = fn(trx)
}

func (c *Core) Tools() tools.ITools {
	return c.tools
}

func (c *Core) Id() string {
	return c.id
}

func (c *Core) OwnerId() string {
	return c.ownerId
}

func (c *Core) Gods() []string {
	return c.gods
}

func (c *Core) IpAddr() string {
	return c.Ip
}

func (c *Core) AppPendingTrxs() {
	elpisTrxs := []*worker.Trx{}
	wasmTrxs := []*worker.Trx{}
	for _, trx := range c.appPendingTrxs {
		if trx.Runtime == "elpis" {
			elpisTrxs = append(elpisTrxs, trx)
		} else if trx.Runtime == "wasm" {
			wasmTrxs = append(wasmTrxs, trx)
		}
	}
	if len(elpisTrxs) > 0 {
		c.Tools().Elpis().ExecuteChainTrxsGroup(elpisTrxs)
	}
	if len(wasmTrxs) > 0 {
		c.Tools().Wasm().ExecuteChainTrxsGroup(wasmTrxs)
	}
	c.appPendingTrxs = []*worker.Trx{}
}

func (c *Core) ClearAppPendingTrxs() {
	c.appPendingTrxs = []*worker.Trx{}
}

func (c *Core) SignPacket(data []byte) string {
	hashed := sha256.Sum256(data)
	signature, err := rsa.SignPSS(rand.Reader, c.privKey, cryp.SHA256, hashed[:], &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
	})
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(signature)
}

func (c *Core) SignPacketAsOwner(data []byte) string {
	hashed := sha256.Sum256(data)
	signature, err := rsa.SignPSS(rand.Reader, c.ownerPrivKey, cryp.SHA256, hashed[:], &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
	})
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(signature)
}

func (c *Core) PlantChainTrigger(count int, userId string, tag string, machineId string, pointId string, attachment string) {
	c.triggerLock.Lock()
	defer c.triggerLock.Unlock()
	c.ModifyState(false, func(trx trx.ITrx) error {
		tail := crypto.SecureUniqueString()
		found := (len(trx.GetByPrefix("chainCallback::"+userId+"_"+tag+"|>")) > 0)
		trx.PutBytes("chainCallback::"+userId+"_"+tag+"|>"+tail, []byte{0x01})
		trx.PutBytes("chainCallback::"+userId+"_"+tag+"|"+tail+"::machineId", []byte(machineId))
		trx.PutBytes("chainCallback::"+userId+"_"+tag+"|"+tail+"::pointId", []byte(pointId))
		trx.PutBytes("chainCallback::"+userId+"_"+tag+"|"+tail+"::attachment", []byte(attachment))
		if !found {
			targetCountB := make([]byte, 4)
			binary.BigEndian.PutUint32(targetCountB, uint32(count))
			trx.PutBytes("chainCallback::"+userId+"_"+tag+"::targetCount", targetCountB)
			tempCountB := make([]byte, 4)
			binary.BigEndian.PutUint32(tempCountB, uint32(0))
			trx.PutBytes("chainCallback::"+userId+"_"+tag+"::tempCount", tempCountB)
		}
		return nil
	})
}

func (c *Core) ExecBaseRequestOnChain(key string, payload []byte, signature string, userId string, tag string, callback func([]byte, int, error)) {
	c.lock.Lock()
	defer c.lock.Unlock()
	callbackId := crypto.SecureUniqueString()
	c.chainCallbacks[callbackId] = &chain.ChainCallback{Tag: tag, Fn: callback, Executors: map[string]bool{}, Responses: map[string]string{}}
	future.Async(func() {
		c.chain <- chain.ChainBaseRequest{Tag: tag, Signatures: []string{c.SignPacket(payload), signature}, Submitter: c.id, RequestId: callbackId, Author: "user::" + userId, Key: key, Payload: payload}
	}, false)
}

func (c *Core) SendMessageOnChain(key string, payload []byte, signature string, userId string, receivers map[string]map[string]bool, ReplyTo string, callback func(string, []byte)) {
	c.lock.Lock()
	defer c.lock.Unlock()
	callbackId := crypto.SecureUniqueString()
	if callback != nil {
		c.messageCallbacks[callbackId] = &chain.MessageCallback{Id: callbackId, Fn: callback}
	}
	future.Async(func() {
		c.chain <- chain.ChainMessage{Key: key, Recievers: receivers, Signatures: []string{c.SignPacket(payload), signature}, Submitter: c.id, RequestId: callbackId, Author: "user::" + userId, Payload: payload}
	}, false)
}

func (c *Core) ExecBaseResponseOnChain(callbackId string, packet []byte, signature string, resCode int, e string, updates []update.Update, tag string, toUserId string) {
	future.Async(func() {
		sort.Slice(updates, func(i, j int) bool {
			return (updates[i].Typ + ":" + updates[i].Key) < (updates[j].Typ + ":" + updates[j].Key)
		})
		c.chain <- chain.ChainResponse{ToUserId: toUserId, Tag: tag, Signature: signature, Executor: c.id, RequestId: callbackId, ResCode: resCode, Err: e, Payload: packet, Effects: chain.Effects{DbUpdates: updates}}
	}, false)
}

func (c *Core) OnChainPacket(typ string, trxPayload []byte) string {
	c.lock.Lock()
	defer c.lock.Unlock()
	switch typ {
	case "message":
		{
			packet := chain.ChainMessage{}
			err := json.Unmarshal(trxPayload, &packet)
			if err != nil {
				log.Println(err)
				return ""
			}
			if packet.ReplyTo != "" {
				cb, ok := c.messageCallbacks[packet.ReplyTo]
				if !ok {
					return ""
				}
				cb.Fn(packet.Key, packet.Payload)
			} else {
				if _, exists := packet.Recievers["*"]; !exists {
					// TODO on chain consensus
				} else {
					if _, exists := packet.Recievers[c.id]; !exists {
						return ""
					} else {
						machineIds := packet.Recievers[c.id]
						for machineId := range machineIds {
							var runtimeType string
							c.ModifyState(true, func(trx trx.ITrx) error {
								vm := mach_model.Vm{MachineId: machineId}.Pull(trx)
								runtimeType = vm.Runtime
								return nil
							})
							future.Async(func() {
								if runtimeType == "wasm" {
									c.Tools().Wasm().RunVm(machineId, packet.PointId, string(packet.Payload))
								}
							}, false)
						}
					}
				}
			}
			break
		}
	case "base":
		{
			packet := chain.ChainBaseRequest{}
			err := json.Unmarshal(trxPayload, &packet)
			if err != nil {
				log.Println(err)
				return ""
			}
			execs := map[string]bool{}
			for k, v := range c.executors {
				execs[k] = v
			}
			if packet.Submitter == c.id {
				c.chainCallbacks[packet.RequestId].Executors = execs
			} else {
				c.chainCallbacks[packet.RequestId] = &chain.ChainCallback{Fn: nil, Executors: execs, Responses: map[string]string{}}
			}
			if !c.executors[c.Ip] {
				return ""
			}
			userId := ""
			if strings.HasPrefix(packet.Author, "user::") {
				userId = packet.Author[len("user::"):]
			}
			action := c.actionStore.FetchAction(packet.Key)
			if action == nil {
				return ""
			}
			var input input.IInput
			i, err2 := action.(iaction.ISecureAction).ParseInput("chain", packet.Payload)
			if err2 != nil {
				log.Println(err2)
				errText := "input parsing error"
				signature := c.SignPacket([]byte(errText))
				c.ExecBaseResponseOnChain(packet.RequestId, []byte{}, signature, 400, errText, []update.Update{}, packet.Tag, userId)
				return ""
			}
			input = i
			resCode, res, err := action.(iaction.ISecureAction).SecurlyActChain(userId, packet.RequestId, packet.Payload, packet.Signatures[1], input, packet.Submitter, packet.Tag)
			if packet.Author == c.id {
				callback := c.chainCallbacks[packet.RequestId]
				delete(c.chainCallbacks, packet.RequestId)
				if callback.Fn != nil {
					if err == nil {
						resData, err := json.Marshal(res)
						if err != nil {
							log.Println(err)
							callback.Fn([]byte("{}"), resCode, err)
						} else {
							callback.Fn(resData, resCode, nil)
						}
					} else {
						callback.Fn([]byte("{}"), resCode, err)
					}
				}
			}
			break
		}
	}
	return ""
}

func (c *Core) Close() {
	c.tools.Network().Chain().Close()
	c.tools.Storage().KvDb().Close()
	c.tools.Storage().TsDb().Close()
	c.tools.Wasm().CloseKVDB()
}

func (c *Core) MarkAsStarted() {
	c.started = true
}

func (c *Core) Load(gods []string, args map[string]interface{}) {
	c.gods = gods

	sroot := args["storageRoot"].(string)
	bdbPath := args["baseDbPath"].(string)
	adbPath := args["appletDbPath"].(string)
	ldbPath := args["pointLogsDb"].(string)
	srchPath := args["searcherDb"].(string)

	dnFederation := driver_network_fed.FirstStageBackFill(c)
	dstorage := driver_storage.NewStorage(c, sroot, bdbPath, ldbPath, srchPath)
	dsignaler := driver_signaler.NewSignaler(c, dnFederation)
	dsecurity := driver_security.New(c, sroot, dstorage, dsignaler)
	dNetwork := driver_network.NewNetwork(c, dstorage, dsecurity, dsignaler, dnFederation)
	dFile := driver_file.NewFileTool(sroot)
	dDocker := driver_docker.NewDocker(c, sroot, dstorage, dFile)
	dWasm := driver_wasm.NewWasm(c, sroot, dstorage, adbPath, dDocker, dFile)
	dElpis := driver_elpis.NewElpis(c, sroot, dstorage)
	dnFederation.SecondStageForFill(dstorage, dFile, dsignaler)
	dFirectl := driver_firectl.NewFireCtl()

	pemData := dsecurity.FetchKeyPair("server_key")[0]
	block, _ := pem.Decode([]byte(pemData))
	if block == nil || block.Type != "PRIVATE KEY" {
		panic("failed to decode PEM block containing private key")
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}
	c.privKey = privateKey.(*rsa.PrivateKey)

	c.tools = &Tools{
		signaler: dsignaler,
		storage:  dstorage,
		security: dsecurity,
		network:  dNetwork,
		file:     dFile,
		docker:   dDocker,
		firectl:  dFirectl,
		wasm:     dWasm,
		elpis:    dElpis,
	}

	c.tools.Network().Chain().RegisterPipeline(func(b [][]byte, insiderCb func([]byte)) []string {
		machineIds := []string{}
		for _, trx := range b {
			firstIndex := strings.Index(string(trx), "::")
			log.Println(string(trx))
			typ := string(trx[:firstIndex])
			if typ == "nodeJoined" {
				insiderCb(trx)
			} else if typ == ("sharderMap|" + c.id) {
				insiderCb(trx)
			} else {
				r := c.OnChainPacket(typ, trx[firstIndex+2:])
				if r != "" {
					machineIds = append(machineIds, r)
				}
			}
		}
		c.AppPendingTrxs()
		return machineIds
	})

	c.chain = make(chan any, 1)
	future.Async(func() {
		for {
			op := <-c.chain
			typ := ""
			switch op.(type) {
			case chain.ChainBaseRequest:
				{
					typ = "base"
					break
				}
			case chain.ChainMessage:
				{
					typ = "message"
					break
				}
			}
			if typ != "" {
				serialized, err := json.Marshal(op)
				if err == nil {
					log.Println(string(serialized))
					c.tools.Network().Chain().SubmitTrx("main", typ, []byte(typ+"::"+string(serialized)))
				} else {
					log.Println(err)
				}
			}
		}
	}, true)

	future.Async(func() {
		for {
			time.Sleep(time.Duration(1) * time.Second)
			func() {
				defer func() {
					if err := recover(); err != nil {
						log.Println(err)
					}
				}()
				minutes := time.Now().Minute()
				seconds := time.Now().Second()
				if (minutes == 0) && ((seconds >= 0) && (seconds <= 2)) {
					c.DoElection()
					time.Sleep(2 * time.Minute)
				}
			}()
		}
	}, false)
}

func (c *Core) DoElection() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.elecReg = true
	future.Async(func() {
		c.chain <- chain.ChainElectionPacket{
			Type:    "election",
			Key:     "choose-validator",
			Meta:    map[string]any{"phase": "start-reg", "voter": c.Ip},
			Payload: []byte("{}"),
		}
	}, false)
}
