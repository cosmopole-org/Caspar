package ws

import (
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"kasper/src/abstract/adapters/signaler"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/trx"
	"kasper/src/shell/utils/crypto"
	"kasper/src/shell/utils/future"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	iaction "kasper/src/abstract/models/action"

	packetmodel "kasper/src/abstract/models/packet"

	"github.com/lxzan/gws"
	cmap "github.com/orcaman/concurrent-map/v2"
)

type Socket struct {
	Id           string
	Lock         sync.Mutex
	Conn         *gws.Conn
	Buffer       [][]byte
	Ack          bool
	Disconnected bool
	app          core.ICore
	server       *Ws
	userId       string
}

var WsSocketPool = sync.Pool{
	New: func() interface{} {
		return &Socket{}
	},
}

type Ws struct {
	app     core.ICore
	sockets *cmap.ConcurrentMap[string, *Socket]
}

const (
	PingInterval = 5 * time.Second
	PingWait     = 10 * time.Second
)

type Handler struct {
	gws.BuiltinEventHandler
	wsServer *Ws
}

func (c *Handler) OnOpen(conn *gws.Conn) {
	println("new ws client connect")
	socket := WsSocketPool.Get().(*Socket)
	socket.server = c.wsServer
	socket.Id = crypto.SecureUniqueString()
	socket.Buffer = [][]byte{}
	socket.Conn = conn
	socket.app = c.wsServer.app
	socket.Ack = true
	socket.Disconnected = false
	conn.Session().Store("session", socket)
}

func (c *Handler) OnClose(socket *gws.Conn, err error) {}

func (c *Handler) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.SetDeadline(time.Now().Add(PingInterval + PingWait))
	_ = socket.WritePong(nil)
}

func (c *Handler) OnPong(socket *gws.Conn, payload []byte) {}

func (c *Handler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	session, _ := socket.Session().Load("session")
	session.(*Socket).processPacket(message.Bytes()[4:])
}

func (t *Ws) Listen(port int, tlsConfig *tls.Config) {
	future.Async(func() {
		upgrader := gws.NewUpgrader(&Handler{wsServer: t}, &gws.ServerOption{
			ParallelEnabled: true,
			Recovery:        gws.Recovery,
		})
		server := &http.Server{
			Addr: fmt.Sprintf(":%d", port),
			Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				conn, err := upgrader.Upgrade(writer, request)
				if err != nil {
					return
				}
				go func() {
					conn.ReadLoop()
					s, exist := conn.Session().Load("session")
					if exist {
						soc := s.(*Socket)
						soc.Lock.Lock()
						defer soc.Lock.Unlock()
						soc.Disconnected = true
						future.Async(func() {
							time.Sleep(60 * time.Second)
							soc.Lock.Lock()
							defer soc.Lock.Unlock()
							currentSoc, found := soc.server.sockets.Get(soc.userId)
							if found {
								if soc == currentSoc {
									if soc.Disconnected {
										t.sockets.Remove(soc.userId)
										t.app.Tools().Signaler().Listeners().Remove(soc.userId)
										WsSocketPool.Put(soc)
									}
								} else {
									WsSocketPool.Put(soc)
								}
							}
						}, false)
					}
				}()
			}),
			TLSConfig: tlsConfig,
		}
		future.Async(func() {
			err := server.ListenAndServeTLS("", "")
			if err != nil {
				log.Fatalf("failed to listen: %v", err)
			}
		}, false)
		println("Clients' Ws TLS server listening on port ", port)
	}, false)
}

func (t *Socket) writeUpdate(key string, updatePack any, writeRaw bool) {

	println("preparing update...")

	keyBytes := []byte(key)
	keyBytesLen := make([]byte, 4)
	binary.BigEndian.PutUint32(keyBytesLen, uint32(len(keyBytes)))

	var b3 []byte
	if writeRaw {
		b3 = updatePack.([]byte)
	} else {
		var err error
		b3, err = json.Marshal(updatePack)
		if err != nil {
			println(err)
			return
		}
	}

	packet := make([]byte, 1+len(keyBytesLen)+len(keyBytes)+len(b3))
	pointer := 1

	packet[0] = 0x01

	copy(packet[pointer:pointer+len(keyBytesLen)], keyBytesLen[:])
	pointer += len(keyBytesLen)
	copy(packet[pointer:pointer+len(keyBytes)], keyBytes[:])
	pointer += len(keyBytes)

	copy(packet[pointer:pointer+len(b3)], b3[:])
	pointer += len(b3)

	println("appending to buffer...")

	t.Lock.Lock()
	defer t.Lock.Unlock()

	t.Buffer = append(t.Buffer, packet)
	t.pushBuffer()
}

func (t *Socket) writeResponse(requestId string, resCode int, response any, writeRaw bool) {

	println("preparing response...")

	b1 := []byte(requestId)
	b1Len := make([]byte, 4)
	binary.BigEndian.PutUint32(b1Len, uint32(len(b1)))

	b2 := make([]byte, 4)
	binary.BigEndian.PutUint32(b2, uint32(resCode))

	var b3 []byte
	if writeRaw {
		b3 = response.([]byte)
	} else {
		var err error
		b3, err = json.Marshal(response)
		if err != nil {
			println(err)
			return
		}
	}

	packet := make([]byte, 1+len(b1Len)+len(b1)+len(b2)+len(b3))
	pointer := 1

	packet[0] = 0x02

	copy(packet[pointer:pointer+len(b1Len)], b1Len[:])
	pointer += len(b1Len)
	copy(packet[pointer:pointer+len(b1)], b1[:])
	pointer += len(b1)

	copy(packet[pointer:pointer+len(b2)], b2[:])
	pointer += len(b2)

	copy(packet[pointer:pointer+len(b3)], b3[:])
	pointer += len(b3)

	println("appending to buffer...")

	t.Lock.Lock()
	defer t.Lock.Unlock()

	t.Buffer = append(t.Buffer, packet)
	t.pushBuffer()
}

func (t *Socket) pushBuffer() {
	println("pushing buffer to client...", t.Ack, len(t.Buffer))
	if t.Ack {
		if len(t.Buffer) > 0 {
			t.Ack = false
			packetLen := make([]byte, 4)
			binary.BigEndian.PutUint32(packetLen, uint32(len(t.Buffer[0])))
			err := t.Conn.WriteMessage(gws.OpcodeBinary, packetLen)
			if err != nil {
				t.Ack = true
				println(err)
				return
			}
			err = t.Conn.WriteMessage(gws.OpcodeBinary, t.Buffer[0])
			if err != nil {
				t.Ack = true
				println(err)
				return
			}
		}
	}
}

func (t *Socket) processPacket(packet []byte) {
	if len(packet) == 1 && packet[0] == 0x01 {
		send := func() {
			t.Lock.Lock()
			defer t.Lock.Unlock()
			t.Ack = true
			if len(t.Buffer) > 0 {
				t.Buffer = t.Buffer[1:]
				t.pushBuffer()
			}
		}
		send()
		return
	}
	pointer := 0
	signatureLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
	println("signature length:", signatureLength)
	if signatureLength > 20000000 {
		return
	}
	pointer += 4
	signature := string(packet[pointer : pointer+signatureLength])
	pointer += signatureLength
	println("signature:", signature)
	userIdLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
	pointer += 4
	println("userId length:", userIdLength)
	if userIdLength > 20000000 {
		return
	}
	userId := string(packet[pointer : pointer+userIdLength])
	pointer += userIdLength
	println("userId:", userId)
	pathLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
	pointer += 4
	println("path length:", pathLength)
	if pathLength > 20000000 {
		return
	}
	path := string(packet[pointer : pointer+pathLength])
	pointer += pathLength
	println("path:", path)
	packetIdLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
	pointer += 4
	println("packetId length:", packetIdLength)
	if packetIdLength > 20000000 {
		return
	}
	packetId := string(packet[pointer : pointer+packetIdLength])
	pointer += packetIdLength
	println("packetId:", packetId)
	payload := packet[pointer:]
	println(string(payload))

	if path == "logout" {
		success, _, _ := t.app.Tools().Security().AuthWithSignature(userId, payload, signature)
		if success {
			t.app.Tools().Signaler().Listeners().Remove(userId)
			t.writeResponse(packetId, 0, packetmodel.BuildErrorJson("loggedout"), false)
		} else {
			t.writeResponse(packetId, 0, packetmodel.BuildErrorJson("logout_failed"), false)
		}
	} else if path == "authenticate" {
		var lis *signaler.Listener
		success, _, _ := t.app.Tools().Security().AuthWithSignature(userId, payload, signature)
		if success {
			func() {
				soc, found := t.server.sockets.Get(userId)
				if !found {
					lis = &signaler.Listener{
						Id:      userId,
						Paused:  false,
						DisTime: 0,
						Signal: func(key string, b any) {
							if b != nil {
								t.writeUpdate(key, b, true)
							}
						},
					}
					t.Ack = true
					return
				}
				soc.Lock.Lock()
				defer soc.Lock.Unlock()
				t.Buffer = soc.Buffer
				lis = &signaler.Listener{
					Id:      userId,
					Paused:  false,
					DisTime: 0,
					Signal: func(key string, b any) {
						if b != nil {
							t.writeUpdate(key, b, true)
						}
					},
				}
				t.Ack = true
			}()
			t.server.sockets.Set(userId, t)
			t.userId = userId
			var pointIds []string
			prefix := "memberof::" + userId + "::"
			t.app.ModifyState(true, func(trx trx.ITrx) error {
				pIds, err := trx.GetLinksList(prefix, -1, -1)
				if err != nil {
					println(err)
					pointIds = []string{}
				} else {
					pointIds = pIds
				}
				return nil
			})
			for _, pointId := range pointIds {
				t.app.Tools().Signaler().JoinGroup(pointId[len(prefix):], userId)
			}
			t.writeResponse(packetId, 0, packetmodel.BuildErrorJson("authenticated"), false)
			t.app.Tools().Signaler().ListenToSingle(lis)
			b, _ := json.Marshal(packetmodel.ResponseSimpleMessage{Message: "old_queue_end"})
			lis.Signal("old_queue_end", b)
		} else {
			t.writeResponse(packetId, 4, packetmodel.BuildErrorJson("authentication failed"), false)
		}
		return
	}
	action := t.app.Actor().FetchAction(path)
	if action == nil {
		t.writeResponse(packetId, 1, packetmodel.BuildErrorJson("action not found"), false)
		return
	}
	var err error
	input, err := action.(iaction.ISecureAction).ParseInput("tcp", payload)
	if err != nil {
		println(err)
		t.writeResponse(packetId, 2, packetmodel.BuildErrorJson(err.Error()), false)
		return
	}
	statusCode, result, err := action.(iaction.ISecureAction).SecurelyAct(userId, packetId, payload, signature, input, strings.Split(t.Conn.RemoteAddr().String(), ":")[0])
	println(result)
	if err != nil {
		httpStatusCode := 3
		if statusCode == -1 {
			httpStatusCode = 4
		}
		t.writeResponse(packetId, httpStatusCode, packetmodel.BuildErrorJson(err.Error()), false)
	}
	t.writeResponse(packetId, 0, result, false)
}

func NewWs(app core.ICore) *Ws {
	m := cmap.New[*Socket]()
	return &Ws{app: app, sockets: &m}
}
