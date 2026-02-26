package tcp

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
	"net"
	"strings"
	"sync"
	"time"

	iaction "kasper/src/abstract/models/action"

	packetmodel "kasper/src/abstract/models/packet"

	cmap "github.com/orcaman/concurrent-map/v2"
)

type Socket struct {
	Id           string
	Lock         sync.Mutex
	Conn         net.Conn
	Buffer       [][]byte
	Ack          bool
	Disconnected bool
	app          core.ICore
	server       *Tcp
	userId       string
}

type Tcp struct {
	app     core.ICore
	sockets *cmap.ConcurrentMap[string, *Socket]
}

func (t *Tcp) Listen(port int, tlsConfig *tls.Config) {
	future.Async(func() {
		ln, err := tls.Listen("tcp", fmt.Sprintf(":%d", port), tlsConfig)
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
		defer ln.Close()
		log.Println("Clients' Tcp TLS server listening on port ", port)

		for {
			conn, err := ln.Accept()
			if err != nil {
				fmt.Println(err)
				continue
			}
			log.Println("new client connected")
			future.Async(func() { t.handleConnection(conn) }, false)
		}
	}, true)
}

func (t *Tcp) listenForPackets(socket *Socket) {
	defer func() {
		t.sockets.Remove(socket.Id)
		socket.Conn.Close()
	}()
	origin := strings.Split(socket.Conn.RemoteAddr().String(), ":")[0]
	lenBuf := make([]byte, 4)
	buf := make([]byte, 1024)
	nextBuf := make([]byte, 2048)
	readCount := 0
	oldReadCount := 0
	enough := false
	beginning := true
	length := 0
	readLength := 0
	remainedReadLength := 0
	var readData []byte
	for {
		if !enough {
			var err error
			readLength, err = socket.Conn.Read(buf)
			if err != nil {
				log.Println(origin, err)
				log.Println(origin, "socket had error and closed")
				return
			}
			func() {
				socket.Lock.Lock()
				defer socket.Lock.Unlock()
				log.Println(origin, "stat 0: reading data...")

				log.Println(origin, buf[0:readLength])

				readCount += readLength
				copy(nextBuf[remainedReadLength:remainedReadLength+readLength], buf[0:readLength])
				remainedReadLength += readLength

				log.Println(origin, nextBuf[0:readLength])

				log.Println(origin, "stat 1:", readLength, oldReadCount, readCount, remainedReadLength)
			}()
		}

		if beginning {
			if readCount >= 4 {
				func() {
					socket.Lock.Lock()
					defer socket.Lock.Unlock()
					log.Println(origin, "stating stat 2...")
					copy(lenBuf, nextBuf[0:4])
					log.Println(origin, "nextBuf", nextBuf[0:4])
					log.Println(origin, "lenBuf", lenBuf[0:4])
					remainedReadLength -= 4
					copy(nextBuf[0:remainedReadLength], nextBuf[4:remainedReadLength+4])
					length = int(binary.BigEndian.Uint32(lenBuf))
					if length > 20000000 {
						return
					}
					readData = make([]byte, length)
					readCount -= 4
					beginning = false
					enough = true

					log.Println(origin, "stat 2:", remainedReadLength, length, readCount)
				}()

			} else {
				enough = false
			}
		} else {
			if remainedReadLength == 0 {
				enough = false
			} else if readCount >= length {
				func() {
					socket.Lock.Lock()
					defer socket.Lock.Unlock()
					log.Println(origin, "stating stat 3...")
					log.Println(origin, "stat 3 step 1", oldReadCount, length)
					copy(readData[oldReadCount:length], nextBuf[0:length-oldReadCount])
					log.Println(origin, "stat 3 step 2", readLength, readCount, length)
					readCount -= length
					copy(nextBuf[0:readCount], nextBuf[length-oldReadCount:(length-oldReadCount)+readCount])
					log.Println(origin, "nextBuf", nextBuf[0:readCount])
					log.Println(origin, "stat 3 step 3", readCount, length)
					remainedReadLength = readCount
					log.Println(origin, "packet received")
					packet := make([]byte, length)
					copy(packet, readData)
					log.Println(origin, "stat 3 step 4")
					oldReadCount = 0
					enough = true
					beginning = true

					log.Println(origin, "stat 3:", remainedReadLength, oldReadCount, readCount)

					future.Async(func() {
						socket.processPacket(packet)
					}, false)
				}()
			} else {
				func() {
					socket.Lock.Lock()
					defer socket.Lock.Unlock()
					log.Println(origin, "stating stat 4...")

					copy(readData[oldReadCount:oldReadCount+(readCount-oldReadCount)], nextBuf[0:readCount-oldReadCount])
					remainedReadLength = 0
					oldReadCount = readCount
					enough = true

					log.Println(origin, "stat 4:", remainedReadLength)
				}()
			}
		}
	}
}

func (t *Tcp) handleConnection(conn net.Conn) {
	socket := &Socket{server: t, Id: crypto.SecureUniqueString(), Buffer: [][]byte{}, Conn: conn, app: t.app, Ack: true, Disconnected: false}
	t.sockets.Set(strings.Split(conn.RemoteAddr().String(), ":")[0], socket)
	future.Async(func() {
		t.listenForPackets(socket)
		socket.Lock.Lock()
		defer socket.Lock.Unlock()
		socket.Disconnected = true
		future.Async(func() {
			time.Sleep(60 * time.Second)
			socket.Lock.Lock()
			defer socket.Lock.Unlock()
			currentSoc, found := socket.server.sockets.Get(socket.userId)
			if found {
				if currentSoc.Disconnected {
					t.sockets.Remove(currentSoc.userId)
					t.app.Tools().Signaler().Listeners().Remove(currentSoc.userId)
				}
			}
		}, false)
	}, false)
}

func (t *Socket) writeUpdate(key string, updatePack any, writeRaw bool) {

	log.Println("preparing update...")

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
			log.Println(err)
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

	log.Println("appending to buffer...")

	t.Lock.Lock()
	defer t.Lock.Unlock()

	t.Buffer = append(t.Buffer, packet)
	t.pushBuffer()
}

func (t *Socket) writeResponse(requestId string, resCode int, response any, writeRaw bool) {

	log.Println("preparing response...")

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
			log.Println(err)
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

	log.Println("appending to buffer...")

	t.Lock.Lock()
	defer t.Lock.Unlock()

	t.Buffer = append(t.Buffer, packet)
	t.pushBuffer()
}

func (t *Socket) pushBuffer() {
	log.Println("pushing buffer to client...", t.Ack, len(t.Buffer))
	if t.Ack {
		if len(t.Buffer) > 0 {
			t.Ack = false
			packetLen := make([]byte, 4)
			binary.BigEndian.PutUint32(packetLen, uint32(len(t.Buffer[0])))
			_, err := t.Conn.Write(packetLen)
			if err != nil {
				t.Ack = true
				log.Println(err)
				return
			}
			_, err = t.Conn.Write(t.Buffer[0])
			if err != nil {
				t.Ack = true
				log.Println(err)
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
	log.Println("signature length:", signatureLength)
	if signatureLength > 20000000 {
		return
	}
	pointer += 4
	signature := string(packet[pointer : pointer+signatureLength])
	pointer += signatureLength
	log.Println("signature:", signature)
	userIdLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
	pointer += 4
	log.Println("userId length:", userIdLength)
	if userIdLength > 20000000 {
		return
	}
	userId := string(packet[pointer : pointer+userIdLength])
	pointer += userIdLength
	log.Println("userId:", userId)
	pathLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
	pointer += 4
	log.Println("path length:", pathLength)
	if pathLength > 20000000 {
		return
	}
	path := string(packet[pointer : pointer+pathLength])
	pointer += pathLength
	log.Println("path:", path)
	packetIdLength := int(binary.BigEndian.Uint32(packet[pointer : pointer+4]))
	pointer += 4
	log.Println("packetId length:", packetIdLength)
	if packetIdLength > 20000000 {
		return
	}
	packetId := string(packet[pointer : pointer+packetIdLength])
	pointer += packetIdLength
	log.Println("packetId:", packetId)
	payload := packet[pointer:]
	log.Println(string(payload))

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
					log.Println(err)
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
		log.Println(err)
		t.writeResponse(packetId, 2, packetmodel.BuildErrorJson(err.Error()), false)
		return
	}
	statusCode, result, err := action.(iaction.ISecureAction).SecurelyAct(userId, packetId, payload, signature, input, strings.Split(t.Conn.RemoteAddr().String(), ":")[0])
	log.Println(result)
	if err != nil {
		httpStatusCode := 3
		if statusCode == -1 {
			httpStatusCode = 4
		}
		t.writeResponse(packetId, httpStatusCode, packetmodel.BuildErrorJson(err.Error()), false)
	}
	t.writeResponse(packetId, 0, result, false)
}

func NewTcp(app core.ICore) *Tcp {
	m := cmap.New[*Socket]()
	return &Tcp{app: app, sockets: &m}
}
