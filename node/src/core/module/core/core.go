package module_core

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
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
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/info"
	"kasper/src/abstract/models/input"
	packetmodel "kasper/src/abstract/models/packet"
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
	"math"
	"slices"

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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"os"

	mathrand "math/rand"

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

func (c *Core) ExecAppletRequestOnChain(pointId string, machineId string, key string, payload []byte, signature string, userId string, tag string, tokenId string, callback func([]byte, int, error)) {
	c.lock.Lock()
	defer c.lock.Unlock()
	callbackId := crypto.SecureUniqueString()
	c.chainCallbacks[callbackId] = &chain.ChainCallback{Tag: tag, Fn: callback, Executors: map[string]bool{}, Responses: map[string]string{}}
	var runtimeType string
	c.ModifyState(true, func(trx trx.ITrx) error {
		vm := mach_model.Vm{MachineId: machineId}.Pull(trx)
		runtimeType = vm.Runtime
		return nil
	})
	future.Async(func() {
		c.chain <- chain.ChainAppletRequest{MachineId: machineId, TokenId: tokenId, Tag: tag, Signatures: []string{c.SignPacket(payload), signature}, Submitter: c.id, RequestId: callbackId, Author: "user::" + userId, Key: key, Payload: payload, Runtime: runtimeType}
	}, false)
}

func (c *Core) ExecAppletResponseOnChain(callbackId string, packet []byte, signature string, resCode int, e string, updates []update.Update) {
	future.Async(func() {
		c.chain <- chain.ChainResponse{Signature: signature, Executor: c.id, RequestId: callbackId, ResCode: resCode, Err: e, Payload: packet, Effects: chain.Effects{DbUpdates: updates}}
	}, false)
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

func (c *Core) SendMessageOnChain(key string, payload []byte, signature string, userId string, receivers []string, ReplyTo string, callback func(string, []byte)) {
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
			if !slices.Contains(packet.Recievers, c.id) {
				return ""
			}
			if packet.Key == "genGlobalId" {
				input := map[string]any{}
				err := json.Unmarshal(packet.Payload, &input)
				if err != nil {
					log.Println(err)
					return ""
				}
				pointIdRaw, ok := input["pointId"]
				if !ok {
					log.Println("pointId not set in chain message")
					return ""
				}
				pointId, ok := pointIdRaw.(string)
				if !ok {
					log.Println("pointId in chain message is not string")
					return ""
				}
				namespaceRaw, ok := input["namespace"]
				if !ok {
					log.Println("namespace not set in chain message")
					return ""
				}
				namespace, ok := namespaceRaw.(string)
				if !ok {
					log.Println("namespace in chain message is not string")
					return ""
				}
				var newValue int64
				c.ModifyState(false, func(trx trx.ITrx) error {
					val := trx.GetBytes(pointId + "::" + namespace)
					if len(val) == 0 {
						newValue = 0
					} else {
						value := binary.LittleEndian.Uint64(val)
						newValue = int64(value) + 1
					}
					newVal := make([]byte, 8)
					binary.LittleEndian.PutUint64(newVal, uint64(newValue))
					trx.PutBytes(pointId+"::"+namespace, newVal)
					return nil
				})
				res, _ := json.Marshal(map[string]any{
					"globalId":  newValue,
					"pointId":   pointId,
					"namespace": namespace,
				})
				signature := c.SignPacketAsOwner(res)
				c.SendMessageOnChain("globalIdGened", res, signature, c.ownerId, []string{packet.Submitter}, packet.RequestId, nil)
			} else if packet.Key == "globalIdGened" {
				cb, ok := c.messageCallbacks[packet.ReplyTo]
				if !ok {
					return ""
				}
				cb.Fn(packet.Key, packet.Payload)
			}
			break
		}
	case "election":
		{
			packet := chain.ChainElectionPacket{}
			err := json.Unmarshal(trxPayload, &packet)
			if err != nil {
				log.Println(err)
				return ""
			}
			if packet.Key == "choose-validator" {
				phaseRaw, ok := packet.Meta["phase"]
				if !ok {
					return ""
				}
				phase, ok2 := phaseRaw.(string)
				if !ok2 {
					return ""
				}
				voterRaw, ok := packet.Meta["voter"]
				if !ok {
					return ""
				}
				voter, ok2 := voterRaw.(string)
				if !ok2 {
					return ""
				}
				if phase == "start-reg" {
					c.elecReg = true
					c.elecStarter = voter
					c.elecStartTime = time.Now().UnixMilli()
					c.elections = []chain.Election{}
					for i := 0; i < MAX_VALIDATOR_COUNT; i++ {
						c.elections = append(c.elections, chain.Election{Participants: map[string]bool{}, Commits: map[string][]byte{}, Reveals: map[string]string{}})
					}
					future.Async(func() {
						c.chain <- chain.ChainElectionPacket{
							Type:    "election",
							Key:     "choose-validator",
							Meta:    map[string]any{"phase": "register", "voter": c.Ip},
							Payload: []byte("{}"),
						}
					}, false)
					if voter == c.Ip {
						future.Async(func() {
							time.Sleep(time.Duration(10) * time.Second)
							c.chain <- chain.ChainElectionPacket{
								Type:    "election",
								Key:     "choose-validator",
								Meta:    map[string]any{"phase": "end-reg", "voter": c.Ip},
								Payload: []byte("{}"),
							}
						}, false)
					}
				} else if phase == "end-reg" {
					if c.elections == nil {
						return ""
					}
					if c.elecStarter == voter && ((time.Now().UnixMilli() - c.elecStartTime) > 8000) {
						c.elecReg = false
						payload := [][]byte{}
						nodeCount := len(c.tools.Network().Chain().Peers())
						hasher := sha256.New()
						for i := 0; i < MAX_VALIDATOR_COUNT; i++ {
							r := fmt.Sprintf("%d", mathrand.Intn(nodeCount))
							c.elections[i].MyNum = r
							hasher.Write([]byte(r))
							bs := hasher.Sum(nil)
							payload = append(payload, bs)
						}
						data, _ := json.Marshal(payload)
						future.Async(func() {
							c.chain <- chain.ChainElectionPacket{
								Type:    "election",
								Key:     "choose-validator",
								Meta:    map[string]any{"phase": "commit", "voter": c.Ip},
								Payload: data,
							}
						}, false)
					}
				} else if phase == "register" {
					if c.elections == nil {
						return ""
					}
					if c.elecReg {
						for i := 0; i < MAX_VALIDATOR_COUNT; i++ {
							c.elections[i].Participants[voter] = true
						}
					}
				} else if phase == "commit" {
					if c.elections == nil {
						return ""
					}
					votes := [][]byte{}
					e := json.Unmarshal(packet.Payload, &votes)
					if e != nil {
						return ""
					}
					if len(votes) < MAX_VALIDATOR_COUNT {
						return ""
					}
					if c.elections[0].Participants[voter] {
						for i := 0; i < min(MAX_VALIDATOR_COUNT, len(c.tools.Network().Chain().Peers())); i++ {
							c.elections[i].Commits[voter] = votes[i]
						}
						if len(c.elections[0].Commits) == len(c.elections[0].Participants) {
							myReveals := []string{}
							for i := 0; i < MAX_VALIDATOR_COUNT; i++ {
								myReveals = append(myReveals, c.elections[i].MyNum)
							}
							data, _ := json.Marshal(myReveals)
							future.Async(func() {
								c.chain <- chain.ChainElectionPacket{
									Type:    "election",
									Key:     "choose-validator",
									Meta:    map[string]any{"phase": "reveal", "voter": c.Ip},
									Payload: data,
								}
							}, false)
						}
					}
				} else if phase == "reveal" {
					if c.elections == nil {
						return ""
					}
					votes := []string{}
					e := json.Unmarshal(packet.Payload, &votes)
					if e != nil {
						return ""
					}
					if len(votes) < MAX_VALIDATOR_COUNT {
						return ""
					}
					if c.elections[0].Participants[voter] {
						for i := 0; i < min(MAX_VALIDATOR_COUNT, len(c.tools.Network().Chain().Peers())); i++ {
							c.elections[i].Reveals[voter] = votes[i]
						}
						if len(c.elections[0].Reveals) == len(c.elections[0].Participants) {
							c.executors = map[string]bool{}
							nodesArr := []string{}
							for p := range c.elections[0].Participants {
								nodesArr = append(nodesArr, p)
							}
							sort.Strings(nodesArr)
							for _, elec := range c.elections[0:min(MAX_VALIDATOR_COUNT, len(nodesArr))] {
								res := -1
								first := true
								for v := range elec.Participants {
									hasher := sha256.New()
									commit := elec.Commits[v]
									reveal := elec.Reveals[v]
									hasher.Write([]byte(reveal))
									bs := hasher.Sum(nil)
									if !bytes.Equal(bs, commit) {
										continue
									}
									num, e := strconv.ParseInt(reveal, 10, 32)
									if e != nil {
										continue
									}
									if first {
										first = false
										res = int(num)
									} else {
										res ^= int(num)
									}
								}
								result := res % len(nodesArr)
								candidate := nodesArr[result]
								c.executors[candidate] = true
								nodesArr = append(nodesArr[:result], nodesArr[result+1:]...)
								c.elections = nil
							}
						}
					}
				}
			}
			break
		}
	case "baseRequest":
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
			action.(iaction.ISecureAction).SecurlyActChain(userId, packet.RequestId, packet.Payload, packet.Signatures[1], input, packet.Submitter, packet.Tag)
			break
		}
	case "appRequest":
		{
			packet := chain.ChainAppletRequest{}
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
			c.appPendingTrxs = append(c.appPendingTrxs, &worker.Trx{CallbackId: packet.RequestId, Runtime: packet.Runtime, UserId: userId, MachineId: packet.MachineId, Key: packet.Key, Payload: string(packet.Payload)})
			return packet.MachineId
		}
	case "response":
		{
			packet := chain.ChainResponse{}
			err := json.Unmarshal(trxPayload, &packet)
			if err != nil {
				log.Println(err)
				return ""
			}
			callback, ok3 := c.chainCallbacks[packet.RequestId]
			if ok3 {
				if !callback.Executors[packet.Executor] {
					return ""
				}
				str, _ := json.Marshal(core.ResponseHolder{Payload: packet.Payload, Effects: packet.Effects})
				callback.Responses[packet.Executor] = string(str)
				if len(callback.Responses) < len(callback.Executors) {
					return ""
				}
				temp := ""
				for _, res := range callback.Responses {
					if temp == "" {
						temp = res
					} else if res != temp {
						temp = ""
						break
					}
				}
				if temp == "" {
					return ""
				}

				kvTokenKeyword := "consumeToken: "
				kvstoreKeyword := "applet: "
				for _, ef := range packet.Effects.DbUpdates {
					if (len(ef.Val) > len(kvTokenKeyword)) && (string(ef.Val[0:len(kvTokenKeyword)]) == kvTokenKeyword) {
						tokenData := packetmodel.ConsumeTokenInput{}
						e := json.Unmarshal([]byte(string(ef.Val[len(kvTokenKeyword):])), &tokenData)
						if e != nil {
							log.Println(e)
							break
						}
						c.ModifyState(false, func(trx trx.ITrx) error {
							user := mach_model.User{Id: tokenData.TokenOwnerId}.Pull(trx)
							if user.Balance < tokenData.Amount {
								err := errors.New("your balance is not enough")
								log.Println(err)
								return err
							}
							if m, e := trx.GetJson("Json::User::"+tokenData.TokenOwnerId, "lockedTokens."+tokenData.TokenId); e == nil {
								amount := int64(m["amount"].(float64))
								validators := []string{}
								json.Unmarshal([]byte(m["validators"].(string)), &validators)
								if !slices.Contains(validators, packet.Executor) {
									err := errors.New("you are not validator")
									log.Println(err)
									return err
								}
								if amount >= tokenData.Amount {
									for _, orig := range validators {
										nodeOwnerId := c.tools.Network().Chain().GetNodeOwnerId(orig)
										toUser := mach_model.User{Id: nodeOwnerId}.Pull(trx)
										toUser.Balance += int64(math.Floor(float64(tokenData.Amount) / float64(len(validators))))
										toUser.Push(trx)
										trx.DelJson("Json::User::"+tokenData.TokenOwnerId, "lockedTokens."+tokenData.TokenId)
									}
									user.Balance += (amount - tokenData.Amount)
									user.Push(trx)
									return nil
								} else {
									err := errors.New("invalid cost value")
									log.Println(err)
									return err
								}
							} else {
								log.Println(e)
								return e
							}
						})
						break
					} else if (len(ef.Val) > len(kvstoreKeyword)) && (string(ef.Val[0:len(kvstoreKeyword)]) == kvstoreKeyword) {
						c.tools.Wasm().ExecuteChainEffects(string(ef.Val[len(kvstoreKeyword):]))
					}
				}

				if !callback.Executors[c.Ip] {
					c.ModifyState(false, func(trx trx.ITrx) error {
						for _, ef := range packet.Effects.DbUpdates {
							if ef.Key != "" {
								if ef.Typ == "put" {
									trx.PutBytes(ef.Key, ef.Val)
								} else if ef.Typ == "del" {
									trx.DelKey(ef.Key)
								}
							}
						}
						return nil
					})
				}
				delete(c.chainCallbacks, packet.RequestId)
				if callback.Fn != nil {
					if packet.Err == "" {
						callback.Fn(packet.Payload, packet.ResCode, nil)
						tempCount := int32(0)
						targetCount := int32(-1)
						keys := []string{}
						c.ModifyState(false, func(trx trx.ITrx) error {
							keys = trx.GetByPrefix("chainCallback::" + packet.ToUserId + "_" + packet.Tag + "|>")
							if len(keys) > 0 {
								keyParts := strings.Split(keys[0], "|>")
								key := keyParts[0]
								countB := trx.GetBytes(key + "::tempCount")
								tempCount = int32(binary.BigEndian.Uint32(countB[:]))
								countB2 := trx.GetBytes(key + "::targetCount")
								targetCount = int32(binary.BigEndian.Uint32(countB2[:]))
								tempCount++
								countBNext := make([]byte, 4)
								binary.BigEndian.PutUint32(countBNext, uint32(tempCount))
								trx.PutBytes(key+"::tempCount", countBNext)
								trx.PutBytes(fmt.Sprintf(key+"::collected_%d", tempCount), packet.Payload)
							}
							return nil
						})
						if tempCount == targetCount {
							pointIds := []string{}
							attachments := []string{}
							payloadsArr := [][]string{}
							c.ModifyState(false, func(trx trx.ITrx) error {
								for _, keyRaw := range keys {
									log.Println(keyRaw)
									keyParts := strings.Split(keyRaw, "|>")
									key := keyParts[0] + "|" + keyParts[1]
									pointId := string(trx.GetBytes(key + "::pointId"))
									attachment := string(trx.GetBytes(key + "::attachment"))
									payloads := []string{}
									for i := 1; i <= int(targetCount); i++ {
										payloads = append(payloads, string(trx.GetBytes(fmt.Sprintf(keyParts[0]+"::collected_%d", i))))
									}
									pointIds = append(pointIds, pointId)
									attachments = append(attachments, attachment)
									payloadsArr = append(payloadsArr, payloads)
									trx.DelKey(keyRaw)
									trx.DelKey(key + "::machineId")
									trx.DelKey(key + "::pointId")
									trx.DelKey(key + "::attachment")
									trx.DelKey(keyParts[0] + "::targetCount")
									trx.DelKey(keyParts[0] + "::tempCount")
								}
								return nil
							})
							for i := 0; i < len(attachments); i++ {
								input := map[string]any{
									"attachment": attachments[i],
									"payloads":   payloadsArr[i],
								}
								b, e := json.Marshal(input)
								if e != nil {
									log.Println(e)
									return ""
								}
								future.Async(func() {
									c.tools.Wasm().RunVm(packet.ToUserId, pointIds[i], string(b))
								}, false)
							}
						}
					} else {
						callback.Fn(packet.Payload, packet.ResCode, errors.New(packet.Err))
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
			machineId := ""
			switch opData := op.(type) {
			case chain.ChainBaseRequest:
				{
					typ = "baseRequest"
					break
				}
			case chain.ChainResponse:
				{
					typ = "response"
					break
				}
			case chain.ChainAppletRequest:
				{
					typ = "appRequest"
					machineId = opData.MachineId
					break
				}
			case chain.ChainElectionPacket:
				{
					typ = "election"
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
					c.tools.Network().Chain().SubmitTrx("main", machineId, typ, []byte(typ+"::"+string(serialized)))
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
