package docker

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"kasper/src/abstract/adapters/docker"
	"kasper/src/abstract/adapters/file"
	"kasper/src/abstract/adapters/signaler"
	"kasper/src/abstract/adapters/storage"
	iaction "kasper/src/abstract/models/action"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/packet"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/models/update"
	"kasper/src/abstract/state"
	"kasper/src/core/module/actor/model/base"
	inputs_points "kasper/src/shell/api/inputs/points"
	inputs_storage "kasper/src/shell/api/inputs/storage"
	models "kasper/src/shell/api/model"
	"kasper/src/shell/utils/crypto"
	"kasper/src/shell/utils/future"
	"maps"
	"net"
	"net/http"
	"strconv"
	"sync"

	"archive/tar"
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	network "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	cmap "github.com/orcaman/concurrent-map/v2"
)

type Docker struct {
	app         core.ICore
	storageRoot string
	storage     storage.IStorage
	file        file.IFile
	lockers     cmap.ConcurrentMap[string, *IOLocker]
	client      *client.Client
}

func (wm *Docker) SaRContainer(machineId string, imageName string, containerName string) {
	cn := strings.Join(strings.Split(machineId, "@"), "_") + "_" + imageName + "_" + containerName
	ctx := context.Background()

	if err := wm.client.ContainerStop(ctx, cn, container.StopOptions{}); err != nil {
		log.Println("Unable to stop container ", cn, err.Error())
	}

	removeOptions := container.RemoveOptions{
		RemoveVolumes: true,
		Force:         true,
	}

	if err := wm.client.ContainerRemove(ctx, cn, removeOptions); err != nil {
		log.Println("Unable to remove container: ", err.Error())
	}
}

func WriteToTar(inputFiles map[string]string) string {
	tarId := crypto.SecureUniqueString()
	buf, err := os.Create(tarId)
	if err != nil {
		log.Println(err)
		return ""
	}
	tw := tar.NewWriter(buf)
	defer func() {
		tw.Close()
		buf.Close()
	}()
	for path, name := range inputFiles {
		fr, _ := os.Open(path)
		defer fr.Close()
		fi, _ := fr.Stat()
		h := new(tar.Header)
		if fi.IsDir() {
			h.Typeflag = tar.TypeDir
		} else {
			h.Typeflag = tar.TypeReg
		}
		h.Name = name
		h.Size = fi.Size()
		h.Mode = int64(fi.Mode())
		h.ModTime = fi.ModTime()
		_ = tw.WriteHeader(h)
		if !fi.IsDir() {
			_, _ = io.Copy(tw, fr)
		}
	}
	return tarId
}

func WriteToTarDirectly(inputFiles map[string][]byte) string {
	tarId := crypto.SecureUniqueString()
	buf, err := os.Create(tarId)
	if err != nil {
		log.Println(err)
		return ""
	}
	tw := tar.NewWriter(buf)
	defer func() {
		tw.Close()
		buf.Close()
	}()
	for name, content := range inputFiles {
		h := new(tar.Header)
		h.Typeflag = tar.TypeReg
		h.Name = name
		h.Size = int64(len(content))
		h.Mode = int64(0600)
		_ = tw.WriteHeader(h)
		_, _ = tw.Write(content)
	}
	return tarId
}

func (wm *Docker) readFromTar(tr *tar.Reader, machineId string, pointId string) (*models.File, error) {
	header, err := tr.Next()

	switch {
	case err == io.EOF:
		return nil, err
	case err != nil:
		return nil, err
	}

	if header.Typeflag == tar.TypeReg {
		var file *models.File
		wm.app.ModifyState(false, func(trx trx.ITrx) error {
			file = &models.File{Id: wm.storage.GenId(trx, wm.app.Id()), OwnerId: machineId, PointId: pointId}
			return nil
		})
		if err := wm.file.SaveTarFileItemToStorage(wm.storageRoot, tr, pointId, file.Id); err != nil {
			log.Println(err)
			return nil, err
		}
		wm.app.ModifyState(false, func(trx trx.ITrx) error {
			file.Push(trx)
			return nil
		})
		return file, nil
	}
	err2 := errors.New("not a file")
	log.Println(err2)
	return nil, err2
}

func (wm *Docker) CopyToContainer(machineId string, imageName string, containerName string, fileName string, content string) error {
	cn := strings.Join(strings.Split(machineId, "@"), "_") + "_" + imageName + "_" + containerName
	tarId := WriteToTarDirectly(map[string][]byte{fileName: []byte(content)})
	tarStream, err := os.Open(tarId)
	if err != nil {
		log.Println(err)
		return err
	}
	ctx := context.Background()
	err = wm.client.CopyToContainer(ctx, cn, "/app/input", tarStream, container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
	})
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

type IOLocker struct {
	Lock sync.Mutex
	conn net.Conn
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

func (wm *Docker) dockerCallback(machineId string, dataRaw string) string {
	defer func() {
		if err := recover(); err != nil {
			log.Println(err)
		}
	}()
	log.Println(dataRaw)
	data := map[string]any{}
	err := json.Unmarshal([]byte(dataRaw), &data)
	if err != nil {
		log.Println(err)
		return err.Error()
	}
	key, err := checkField(data, "key", "")
	if err != nil {
		log.Println(err)
		return err.Error()
	}
	input, err := checkField[map[string]any](data, "input", nil)
	if err != nil {
		log.Println(err)
		return err.Error()
	}
	if key == "dbOp" {
		op, err := checkField(input, "op", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		if op == "putObj" {
			typ, err := checkField(input, "objType", "")
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			id, err := checkField(input, "objId", "")
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			objRaw, err := checkField(input, "obj", map[string]any{})
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			obj := map[string][]byte{}
			for k, v := range objRaw {
				obj[k], err = base64.StdEncoding.DecodeString(v.(string))
				if err != nil {
					log.Println(err)
					return err.Error()
				}
			}
			wm.app.ModifyState(false, func(trx trx.ITrx) error {
				trx.PutObj(machineId+"->"+typ, id, obj)
				return nil
			})
			return "{}"
		} else if op == "getObj" {
			typ, err := checkField(input, "objType", "")
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			id, err := checkField(input, "objId", "")
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			obj := map[string][]byte{}
			wm.app.ModifyState(true, func(trx trx.ITrx) error {
				obj = trx.GetObj(machineId+"->"+typ, id)
				return nil
			})
			otuput, err := json.Marshal(obj)
			if err != nil {
				log.Println(err)
				return "{}"
			}
			return string(otuput)
		} else if op == "getObjsByPrefix" {
			typ, err := checkField(input, "objType", "")
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			prefix, err := checkField(input, "prefix", "")
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			offset, err := checkField(input, "offset", float64(0))
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			count, err := checkField(input, "count", float64(0))
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			result := map[string]map[string][]byte{}
			wm.app.ModifyState(true, func(trx trx.ITrx) error {
				links, err := trx.GetLinksList(machineId+"->"+prefix, int(offset), int(count))
				if err != nil {
					log.Println(err)
					return nil
				}
				for i := range len(links) {
					links[i] = links[i][len(machineId+"->"+prefix):]
				}
				res, err := trx.GetObjList(machineId+"->"+typ, links, map[string]string{})
				result = res
				return err
			})
			str, _ := json.Marshal(result)
			return string(str)
		} else if op == "getObjs" {
			typ, err := checkField(input, "objType", "")
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			offset, err := checkField(input, "offset", float64(0))
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			count, err := checkField(input, "count", float64(0))
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			result := map[string]map[string][]byte{}
			wm.app.ModifyState(true, func(trx trx.ITrx) error {
				res, err := trx.GetObjList(machineId+"->"+typ, []string{"*"}, map[string]string{}, int64(offset), int64(count))
				result = res
				return err
			})
			str, _ := json.Marshal(result)
			return string(str)
		} else if op == "putLink" {
			k, err := checkField(input, "key", "")
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			v, err := checkField(input, "val", "")
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			wm.app.ModifyState(false, func(trx trx.ITrx) error {
				trx.PutLink(machineId+"->"+k, v)
				return nil
			})
			return "{}"
		}
	} else if key == "runDocker" {
		pointId, err := checkField(input, "pointId", "")
		if err != nil {
			log.Println(err)
			return err.Error()
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
			log.Println(err)
			return err.Error()
		}
		inputFilesStr, err := checkField(input, "inputFiles", "{}")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		inputFiles := map[string]string{}
		err = json.Unmarshal([]byte(inputFilesStr), &inputFiles)
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		finalInputFiles := map[string]string{}
		for k, v := range inputFiles {
			if !wm.file.CheckFileFromStorage(wm.storageRoot, pointId, k) {
				err := errors.New("input file does not exist")
				log.Println(err)
				return err.Error()
			}
			path := fmt.Sprintf("%s/files/%s/%s", wm.storageRoot, pointId, k)
			finalInputFiles[path] = v
		}
		imageName, err := checkField(input, "imageName", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		containerName, err := checkField(input, "containerName", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		wm.SaRContainer(machineId, imageName, containerName)
		outputFile, err := wm.RunContainer(machineId, pointId, imageName, containerName, finalInputFiles, false)
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		if outputFile != nil {
			str, err := json.Marshal(outputFile)
			if err != nil {
				log.Println(err)
				return err.Error()
			}
			return string(str)
		}
	} else if key == "execDocker" {
		imageName, err := checkField(input, "imageName", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		containerName, err := checkField(input, "containerName", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		command, err := checkField(input, "command", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		output, err := wm.ExecContainer(machineId, imageName, containerName, command)
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		return output
	} else if key == "copyToDocker" {
		imageName, err := checkField(input, "imageName", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		containerName, err := checkField(input, "containerName", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		fileName, err := checkField(input, "fileName", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		content, err := checkField(input, "content", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		err = wm.CopyToContainer(machineId, imageName, containerName, fileName, content)
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		return ""
	} else if key == "log" {
		_, err := checkField(input, "text", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		// log.Println("elpis vm:", text)
	} else if key == "httpPost" {
		url, err := checkField(input, "url", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		method := strings.Split(url, "|")[0]
		url = url[len(method)+1:]
		headers, err := checkField(input, "headers", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		body, err := checkField(input, "body", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		req, err := http.NewRequest(method, url, bytes.NewBuffer([]byte(body)))
		if err != nil {
			log.Println("Error creating request:" + err.Error())
			return err.Error()
		}
		heads := map[string]string{}
		err = json.Unmarshal([]byte(headers), &heads)
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		for k, v := range heads {
			req.Header.Set(k, v)
		}
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Println("Request failed:" + err.Error())
			return err.Error()
		}
		defer resp.Body.Close()
		log.Println("Response status:" + resp.Status)
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println("Error reading response body:" + err.Error())
			return err.Error()
		}
		return base64.StdEncoding.EncodeToString(bodyBytes)
	} else if key == "checkTokenValidity" {
		tokenOwnerId, err := checkField(input, "tokenOwnerId", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		tokenId, err := checkField(input, "tokenId", "")
		if err != nil {
			log.Println(err)
			return err.Error()
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
		return string(jsn)
	} else if key == "submitOnchainResponse" {
		callbackId, err := checkField(input, "callbackId", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		cost, err := checkField[float64](input, "cost", 0)
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		tokenOwnerId, err := checkField(input, "tokenOwnerId", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		tokenId, err := checkField(input, "tokenId", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		pack, err := checkField(input, "packet", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		changes, err := checkField(input, "changes", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		resCode, err := checkField[float64](input, "resCode", 0)
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		e, err := checkField(input, "error", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		trxInp := packet.ConsumeTokenInput{TokenId: tokenId, Amount: int64(cost), TokenOwnerId: tokenOwnerId}
		i, _ := json.Marshal(trxInp)
		wm.app.ModifyState(false, func(trx trx.ITrx) error {
			trx.PutString("Temp::User::"+tokenOwnerId+"::consumedTokens::"+tokenId, "true")
			return nil
		})
		wm.app.ExecAppletResponseOnChain(callbackId, []byte(pack), "#appletsign", int(resCode), e, []update.Update{{Val: []byte("consumeToken: " + string(i))}, {Val: []byte("applet: " + changes)}})
	} else if key == "submitOnchainTrx" {
		targetMachineId, err := checkField(input, "targetMachineId", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		isRequesterOnchain := false
		kRaw, err := checkField(input, "key", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		kParts := strings.Split(kRaw, "|")
		dstPointId := kParts[0]
		srcPointId, err := checkField(input, "pointId", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		k := kParts[1]
		userId := kParts[2]
		userSignature := kParts[3]
		tokenId := kParts[4]
		onchainReq := kParts[5] == "true"
		isFile, err := checkField(input, "isFile", false)
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		isBase, err := checkField(input, "isBase", false)
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		tag, err := checkField(input, "tag", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		pack, err := checkField(input, "packet", "{}")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		var data []byte
		if isFile {
			if wm.file.CheckFileFromStorage(wm.storageRoot, srcPointId, pack) {
				b, err := wm.file.ReadFileFromStorage(wm.storageRoot, srcPointId, pack)
				if err != nil {
					log.Println(err)
					return err.Error()
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
						log.Println(err)
						return
					}
					result = b
					if isRequesterOnchain {
						outputCnan <- 1
					}
				})
			} else {
				action := wm.app.Actor().FetchAction(k)
				if action == nil {
					return "action not found"
				}
				var err error
				inp, err := action.(iaction.ISecureAction).ParseInput("tcp", data)
				if err != nil {
					log.Println(err)
					return err.Error()
				}
				_, result, err := action.(iaction.ISecureAction).SecurelyAct(userId, "", data, userSignature, inp, "", true)
				log.Println(result)
				if err != nil {
					return err.Error()
				}
				str, _ := json.Marshal(result)
				return string(str)
			}
		} else {
			if onchainReq {
				wm.app.ExecAppletRequestOnChain(dstPointId, targetMachineId, k, data, userSignature, userId, tag, tokenId, func(b []byte, i int, err error) {
					if err != nil {
						log.Println(err)
						return
					}
					result = b
					if isRequesterOnchain {
						outputCnan <- 1
					}
				})
			}
		}
		if isRequesterOnchain {
			<-outputCnan
		}
		if isRequesterOnchain {
			return string(result)
		} else {
			return "{}"
		}
	} else if key == "plantTrigger" {
		count, err := checkField(input, "count", float64(0))
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		tag, err := checkField(input, "tag", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		pointId, err := checkField(input, "pointId", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		data, err := checkField(input, "input", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		if tag != "alarm" {
			wm.app.PlantChainTrigger(int(count), machineId, tag, machineId, pointId, data)
		}
	} else if key == "signalPoint" {
		typAndTemp, err := checkField(input, "type", "")
		if err != nil {
			log.Println(err)
			return err.Error()
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
			log.Println(err)
			return err.Error()
		}
		userId, err := checkField(input, "userId", "")
		if err != nil {
			log.Println(err)
			return err.Error()
		}
		data, err := checkField(input, "data", "")
		if err != nil {
			log.Println(err)
			return err.Error()
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

	return "{}"
}

func (wm *Docker) Assign(machineId string) {
	wm.lockers.SetIfAbsent(machineId, &IOLocker{})
	wm.app.Tools().Signaler().ListenToSingle(&signaler.Listener{
		Id: machineId,
		Signal: func(key string, a any) {
			if key == "points/signal" {
				data := a.([]byte)
				locker, found := wm.lockers.Get(machineId)
				if found && locker.conn != nil {
					locker.Lock.Lock()
					defer locker.Lock.Unlock()
					lenBytes := make([]byte, 4)
					binary.LittleEndian.PutUint32(lenBytes, uint32(len(data)))
					locker.conn.Write(lenBytes)
					cbBytes := make([]byte, 8)
					binary.LittleEndian.PutUint64(cbBytes, uint64(0))
					locker.conn.Write(cbBytes)
					locker.conn.Write(data)
				}
			}
		},
	})
}

func (wm *Docker) RunContainer(machineId string, pointId string, imageName string, containerName string, inputFile map[string]string, standalone bool) (*models.File, error) {

	cn := strings.Join(strings.Split(machineId, "@"), "_") + "_" + imageName + "_" + containerName

	ctx := context.Background()

	config := &container.Config{
		Image: strings.Join(strings.Split(machineId, "@"), "_") + "/" + imageName,
		Env:   []string{},
	}

	_, err := wm.client.ContainerCreate(
		ctx,
		config,
		&container.HostConfig{
			LogConfig: container.LogConfig{
				Type:   "json-file",
				Config: map[string]string{},
			},
			Runtime:     "runsc",
			NetworkMode: "kasper",
		},
		nil,
		nil,
		cn,
	)

	if err != nil {
		log.Println(err)
		return nil, err
	}
	defer wm.SaRContainer(machineId, imageName, containerName)
	if !standalone {
		future.Async(func() {
			time.Sleep(60 * time.Minute)
			wm.SaRContainer(machineId, imageName, containerName)
		}, false)
	}

	tarId := WriteToTar(inputFile)
	tarStream, err := os.Open(tarId)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	err = wm.client.CopyToContainer(ctx, cn, "/app/input", tarStream, container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: true,
	})
	if err != nil {
		log.Println(err)
		return nil, err
	}

	err = wm.client.ContainerStart(ctx, cn, container.StartOptions{})
	if err != nil {
		log.Println(err)
		return nil, err
	}

	log.Println("Container ", cn, " is created")

	waiter, err := wm.client.ContainerAttach(ctx, cn, container.AttachOptions{
		Stderr: true,
		Stdout: true,
		Stdin:  true,
		Stream: true,
	})

	go io.Copy(os.Stdout, waiter.Reader)
	go io.Copy(os.Stderr, waiter.Reader)

	if err != nil {
		log.Println(err)
		return nil, err
	}

	statusCh, errCh := wm.client.ContainerWait(ctx, cn, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			log.Println(err)
			return nil, err
		}
	case <-statusCh:
	}
	if !standalone {
		reader, _, err := wm.client.CopyFromContainer(ctx, cn, "/app/output")
		if err != nil {
			log.Println(err)
			return nil, nil
		}
		defer reader.Close()
		r := tar.NewReader(reader)
		file, err := wm.readFromTar(r, machineId, pointId)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		return file, nil
	} else {
		return nil, nil
	}
}

func (wm *Docker) BuildImage(dockerfile string, machineId string, imageName string, outputChan chan string) error {
	ctx := context.Background()

	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	defer tw.Close()

	dockerFileReader, err := os.Open(dockerfile + "/Dockerfile")
	if err != nil {
		return err
	}
	defer dockerFileReader.Close()
	readDockerFile, err := ioutil.ReadAll(dockerFileReader)
	if err != nil {
		return err
	}
	tarHeader := &tar.Header{
		Name: dockerfile + "/Dockerfile",
		Size: int64(len(readDockerFile)),
	}
	err = tw.WriteHeader(tarHeader)
	if err != nil {
		return err
	}
	_, err = tw.Write(readDockerFile)
	if err != nil {
		return err
	}

	files, err := ioutil.ReadDir(dockerfile)
	if err != nil {
		log.Println(err)
		return err
	}
	for _, file := range files {
		err := func() error {
			reader, err := os.Open(dockerfile + "/" + file.Name())
			if err != nil {
				return err
			}
			defer reader.Close()
			readFile, err := ioutil.ReadAll(reader)
			if err != nil {
				return err
			}
			tarHeader := &tar.Header{
				Name: file.Name(),
				Size: int64(len(readFile)),
			}
			err = tw.WriteHeader(tarHeader)
			if err != nil {
				return err
			}
			_, err = tw.Write(readFile)
			if err != nil {
				return err
			}
			return nil
		}()
		if err != nil {
			log.Println(err)
			return err
		}
	}

	dockerFileTarReader := bytes.NewReader(buf.Bytes())

	buildOptions := types.ImageBuildOptions{
		Context:    dockerFileTarReader,
		Dockerfile: dockerfile + "/Dockerfile",
		Remove:     true,
		Tags:       []string{strings.Join(strings.Split(machineId, "@"), "_") + "/" + imageName},
	}

	imageBuildResponse, err := wm.client.ImageBuild(
		ctx,
		dockerFileTarReader,
		buildOptions,
	)

	if err != nil {
		return err
	}

	defer imageBuildResponse.Body.Close()

	scanner := bufio.NewScanner(imageBuildResponse.Body)
	for scanner.Scan() {
		outputChan <- scanner.Text()
	}
	outputChan <- ""

	return nil
}

func (wm *Docker) ExecContainer(machineId string, imageName string, containerName string, command string) (string, error) {

	cn := strings.Join(strings.Split(machineId, "@"), "_") + "_" + imageName + "_" + containerName

	ctx := context.Background()

	config := container.ExecOptions{
		AttachStderr: true,
		AttachStdout: true,
		Cmd:          strings.Split(command, " "),
	}

	res, err := wm.client.ContainerExecCreate(ctx, cn, config)
	if err != nil {
		return "", err
	}
	execId := res.ID

	resp, err := wm.client.ContainerExecAttach(ctx, execId, container.ExecAttachOptions{})
	if err != nil {
		return "", err
	}
	defer resp.Close()

	var outBuf, errBuf bytes.Buffer
	outputDone := make(chan error)

	go func() {
		// StdCopy demultiplexes the stream into two buffers
		_, err = stdcopy.StdCopy(&outBuf, &errBuf, resp.Reader)
		outputDone <- err
	}()

	select {
	case err := <-outputDone:
		if err != nil {
			return "", err
		}
		break

	case <-ctx.Done():
		return "", ctx.Err()
	}

	stdout, err := ioutil.ReadAll(&outBuf)
	if err != nil {
		return "", err
	}
	stderr, err := ioutil.ReadAll(&errBuf)
	if err != nil {
		return "", err
	}

	log.Println("output of exec :", string(stdout))

	return string(stdout) + string(stderr), nil
}

func getContainerNameByIP(ip string, cli *client.Client) string {
	ctx := context.Background()

	// Get a list of all Docker networks.
	networks, err := cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		log.Println("Failed to list Docker networks:", err)
		return ""
	}

	// Iterate through each network to inspect its connected containers.
	for _, net := range networks {
		log.Println(net.Name)
		netInspect, err := cli.NetworkInspect(ctx, net.ID, network.InspectOptions{})
		if err != nil {
			log.Println("Failed to inspect network:", err)
			continue
		}
		log.Println(net.Name)

		// Look for a container on this network that has the matching IP.
		for _, container := range netInspect.Containers {
			log.Println(container.Name)
			// Check if the container's IPv4 address matches the client's IP.
			if strings.Split(container.IPv4Address, "/")[0] == ip {
				log.Println(container.Name)
				// The container name is prefixed with a slash; remove it.
				return strings.TrimPrefix(container.Name, "/")
			}
		}
	}
	return ""
}

func genProxyConfig() string {
	var proxyConfig = `
	events {}
	http {
		# This server block listens for HTTP traffic on port 80
		server {
		    listen 8082;
		    listen [::]:8082;
		    server_name localhost;
 		    return 301 https://$host$request_uri;
		}

		# This server block handles HTTPS traffic on port 443
		server {
		    listen 8443 ssl http2;
    		listen [::]:8443 ssl http2;
    		server_name localhost;

		    # Add your SSL certificate paths and other configurations here
    		ssl_certificate /etc/nginx/ssl/nginx-selfsigned.crt;
    		ssl_certificate_key /etc/nginx/ssl/nginx-selfsigned.key;
			
`
	for machId := range activeMachines {
		proxyConfig += `
			location /` + strings.Join(strings.Split(machId, "@"), "_") + `/hello/ {
            	proxy_pass http://` + strings.Join(strings.Split(machId, "@"), "_") + "_main_main" + `:80/hello;
			}
        	location /` + strings.Join(strings.Split(machId, "@"), "_") + `/stream/get/ {
            	proxy_pass http://` + strings.Join(strings.Split(machId, "@"), "_") + "_main_main" + `:80/stream_get;
			}
			location /` + strings.Join(strings.Split(machId, "@"), "_") + `/stream/send/ {
            	proxy_pass http://` + strings.Join(strings.Split(machId, "@"), "_") + "_main_main" + `:80/stream_send;
			}`
	}
	for machId := range sideMachines {
		parts := strings.Split(machId, "_")
		mi := strings.Join(strings.Split(parts[0], "@"), "_")
		in := parts[1]
		cn := parts[2]
		proxyConfig += `
			location /` + mi + "_" + in + "_" + cn + `/ {
            	proxy_pass http://` + mi + "_" + in + "_" + cn + `:80;
			}`
	}
	proxyConfig += `
		}
	}`
	return proxyConfig
}

var activeMachines = map[string]bool{}
var sideMachines = map[string]bool{}

func registerRoute(mux *http.ServeMux, path string, handler func(w http.ResponseWriter, r *http.Request)) {
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			r := recover()
			if r != nil {
				var err error
				switch t := r.(type) {
				case string:
					err = errors.New(t)
				case error:
					err = t
				default:
					err = errors.New("unknown error")
				}
				log.Println(err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}()
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		handler(w, r)
	})
}

func (wm *Docker) RunGateway() {
	mux := http.NewServeMux()
	portStr := os.Getenv("VM_API_PORT")
	port, err := strconv.ParseInt(portStr, 10, 32)
	if err != nil {
		panic(err)
	}
	server := &http.Server{
		Addr:      fmt.Sprintf(":%d", port),
		Handler:   mux,
		TLSConfig: wm.app.Tools().Network().TlsConfig(),
	}
	registerRoute(mux, "/stream/send", func(w http.ResponseWriter, r *http.Request) {
		userId := r.URL.Query().Get("userId")
		inputBody := []byte(r.URL.Query().Get("input"))
		signature := r.URL.Query().Get("signature")
		if success, _, _ := wm.app.Tools().Security().AuthWithSignature(userId, inputBody, string(signature)); !success {
			log.Println("Error accessing point:", err.Error())
			http.Error(w, "signature verification failed", http.StatusForbidden)
			return
		}
		origin := ""
		wm.app.ModifyState(true, func(trx trx.ITrx) error {
			uParts := strings.Split(string(trx.GetColumn("User", userId, "username")), "@")
			if len(uParts) < 2 {
				return nil
			}
			origin = uParts[1]
			return nil
		})
		if origin == wm.app.Id() {
			var input inputs_storage.StreamGetInput
			err = json.Unmarshal(inputBody, &input)
			if err != nil {
				log.Println("Error parsing body:", err.Error())
				http.Error(w, "can't parse body", http.StatusBadRequest)
				return
			}
			if !wm.app.Tools().Security().HasAccessToPoint(userId, input.PointId) {
				log.Printf("Error accessing point: %v", err)
				http.Error(w, "can't access point", http.StatusForbidden)
				return
			}
			url := fmt.Sprintf("%s://%s%s", "https", "10.10.0.5:8443", "/"+strings.Join(strings.Split(input.MachineId, "@"), "_")+"/stream/send/")
			log.Println(url)

			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				log.Printf("Error reading body: %v", err)
				http.Error(w, "can't read body", http.StatusBadRequest)
				return
			}
			log.Println("len of body in proxy", len(body))
			proxyReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
			if err != nil {
				log.Println(err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			proxyReq.Header = make(http.Header)
			maps.Copy(proxyReq.Header, r.Header)
			proxyReq.Header.Set("Content-Type", "application/octet-stream")
			proxyReq.Header.Set("User-Id", userId)
			proxyReq.Header.Set("Point-Id", input.PointId)
			proxyReq.Header.Set("Metadata", input.Metadata)
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			httpClient := &http.Client{Transport: tr}
			resp, err := httpClient.Do(proxyReq)
			if err != nil {
				log.Println(err)
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			io.Copy(w, resp.Body)
		} else {
			url := fmt.Sprintf("%s://%s%s", "https", origin+":3000", r.RequestURI)
			proxyReq, err := http.NewRequest(r.Method, url, r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			proxyReq.Header = make(http.Header)
			for h, val := range r.Header {
				proxyReq.Header[h] = val
			}
			httpClient := http.Client{}
			resp, err := httpClient.Do(proxyReq)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			resp.Write(w)
		}
	})
	future.Async(func() {
		server.ListenAndServeTLS("", "")
	}, false)
}

func NewDocker(core core.ICore, storageRoot string, storage storage.IStorage, file file.IFile) docker.IDocker {
	client, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Println("Unable to create docker client: ", err.Error())
	}
	wm := &Docker{
		app:         core,
		storageRoot: storageRoot,
		storage:     storage,
		file:        file,
		lockers:     cmap.New[*IOLocker](),
		client:      client,
	}
	future.Async(func() {
		listener, err := net.Listen("tcp", ":8084")
		if err != nil {
			log.Fatalf("Failed to start TCP server: %v", err)
		}
		defer listener.Close()
		log.Println("Docker tcp server listening on :8084")
		for {
			c, err := listener.Accept()
			if err != nil {
				log.Println("Error accepting connection:", err)
				continue
			}
			remoteAddr := c.RemoteAddr().String()
			clientIP := strings.Split(remoteAddr, ":")[0]
			log.Printf("Connection from IP: %s", clientIP)
			containerName := getContainerNameByIP(clientIP, client)
			log.Println(containerName)
			cnParts := strings.Split(containerName, "_")
			machineId := cnParts[0] + "@" + cnParts[1]

			if cnParts[2] == "main" && cnParts[3] == "main" {
				locker, found := wm.lockers.Get(machineId)
				if found {
					locker.conn = c
				}
				activeMachines[machineId] = true
			} else {
				locker, found := wm.lockers.Get(machineId + "_" + cnParts[2] + "_" + cnParts[3])
				if found {
					locker.conn = c
				}
				sideMachines[machineId+"_"+cnParts[2]+"_"+cnParts[3]] = true
			}

			config := genProxyConfig()
			file.SaveDataToGlobalStorage(storageRoot+"/docker_proxy", []byte(config), "nginx.conf", true)

			func() {
				ctx := context.Background()
				config := container.ExecOptions{
					AttachStderr: true,
					AttachStdout: true,
					Cmd:          strings.Split("nginx -s reload", " "),
				}
				res, err := wm.client.ContainerExecCreate(ctx, "kasper-proxy", config)
				if err != nil {
					log.Println(err)
					return
				}
				execId := res.ID
				resp, err := wm.client.ContainerExecAttach(ctx, execId, container.ExecAttachOptions{})
				if err != nil {
					log.Println(err)
					return
				}
				defer resp.Close()
			}()

			future.Async(func() {
				defer func() {
					c.Close()
					log.Printf("docker container disconnected")
				}()

				log.Printf("docker container connected")
				r := bufio.NewReader(c)
				for {
					var ln uint32
					if err := binary.Read(r, binary.LittleEndian, &ln); err != nil {
						if err != io.EOF {
							log.Printf("read len err: %v", err)
						}
						return
					}
					var cbId uint64
					if err := binary.Read(r, binary.LittleEndian, &cbId); err != nil {
						if err != io.EOF {
							log.Printf("read len err: %v", err)
						}
						return
					}
					buf := make([]byte, ln)
					if _, err := io.ReadFull(r, buf); err != nil {
						log.Printf("read body err: %v", err)
						return
					}
					data := []byte(wm.dockerCallback(machineId, string(buf)))
					var locker *IOLocker
					if cnParts[2] == "main" && cnParts[3] == "main" {
						l, found := wm.lockers.Get(machineId)
						if found {
							locker = l
						}
					} else {
						l, found := wm.lockers.Get(machineId + "_" + cnParts[2] + "_" + cnParts[3])
						if found {
							locker = l
						}
					}
					if locker != nil {
						func() {
							locker.Lock.Lock()
							defer locker.Lock.Unlock()
							lenBytes := make([]byte, 4)
							binary.LittleEndian.PutUint32(lenBytes, uint32(len(data)))
							locker.conn.Write(lenBytes)
							callbackId := make([]byte, 8)
							binary.LittleEndian.PutUint64(callbackId, cbId)
							locker.conn.Write(callbackId)
							locker.conn.Write(data)
						}()
					}
				}
			}, false)
		}
	}, false)
	return wm
}
