package model

import (
	"encoding/binary"
	"kasper/src/abstract/models/trx"
	"log"
	"sort"
)

type App struct {
	Id            string `json:"id"`
	ChainId       string `json:"chainId"`
	OwnerId       string `json:"ownerId"`
	Username      string `json:"username"`
	MachinesCount int    `json:"machinesCount"`
	Title         string `json:"title"`
	Avatar        string `json:"avatar"`
	Desc          string `json:"desc"`
}

func (m App) Type() string {
	return "App"
}

func (d App) Push(trx trx.ITrx) {
	mcBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(mcBytes, uint32(d.MachinesCount))
	trx.PutObj(d.Type(), d.Id, map[string][]byte{
		"id":            []byte(d.Id),
		"ownerId":       []byte(d.OwnerId),
		"username":      []byte(d.Username),
		"chainId":       []byte(d.ChainId),
		"machinesCount": mcBytes,
	})
}

func (d App) Pull(trx trx.ITrx, flags ...bool) App {
	m := trx.GetObj(d.Type(), d.Id)
	if len(m) > 0 {
		d.Id = string(m["id"])
		d.Username = string(m["username"])
		d.OwnerId = string(m["ownerId"])
		d.ChainId = string(m["chainId"])
		d.MachinesCount = int(binary.LittleEndian.Uint32(m["machinesCount"]))
		if len(flags) > 0 {
			if flags[0] {
				if metadata, err := trx.GetJson("AppMeta::"+d.Id, "metadata.public.profile"); err == nil {
					d.Title = metadata["title"].(string)
					d.Avatar = metadata["avatar"].(string)
					d.Desc = metadata["desc"].(string)
				}
			}
		}
	}
	return d
}

func (d App) Delete(trx trx.ITrx) {
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::|")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::id")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::username")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::chainId")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::machinesCount")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::ownerId")
	trx.DelJson("AppMeta::"+d.Id, "metadata")
}

func (d App) List(trx trx.ITrx, prefix string) ([]App, error) {
	list, err := trx.GetLinksList(prefix, -1, -1)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	for i := 0; i < len(list); i++ {
		list[i] = list[i][len(prefix):]
	}
	objs, err := trx.GetObjList("App", list, map[string]string{})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []App{}
	for id, m := range objs {
		if len(m) > 0 {
			d := App{}
			d.Id = id
			d.OwnerId = string(m["ownerId"])
			d.Username = string(m["username"])
			d.ChainId = string(m["chainId"])
			d.MachinesCount = int(binary.LittleEndian.Uint32(m["machinesCount"]))
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Id < entities[j].Id
	})
	return entities, nil
}

func (d App) All(trx trx.ITrx, offset int64, count int64) ([]App, error) {
	objs, err := trx.GetObjList("App", []string{"*"}, map[string]string{}, offset, count)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []App{}
	for id, m := range objs {
		if len(m) > 0 {
			d := App{}
			d.Id = id
			d.OwnerId = string(m["ownerId"])
			d.Username = string(m["username"])
			d.ChainId = string(m["chainId"])
			d.MachinesCount = int(binary.LittleEndian.Uint32(m["machinesCount"]))
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Id < entities[j].Id
	})
	return entities, nil
}

type Vm struct {
	MachineId string `json:"id"`
	AppId     string `json:"appId"`
	Runtime   string `json:"runtime"`
	Path      string `json:"path"`
	Comment   string `json:"comment"`
}

func (m Vm) Type() string {
	return "Vm"
}

func (d Vm) Push(trx trx.ITrx) {
	trx.PutObj(d.Type(), d.MachineId, map[string][]byte{
		"machineId": []byte(d.MachineId),
		"appId":     []byte(d.AppId),
		"runtime":   []byte(d.Runtime),
		"path":      []byte(d.Path),
		"comment":   []byte(d.Comment),
	})
}

func (d Vm) Pull(trx trx.ITrx) Vm {
	m := trx.GetObj(d.Type(), d.MachineId)
	if len(m) > 0 {
		d.MachineId = string(m["machineId"])
		d.AppId = string(m["appId"])
		d.Runtime = string(m["runtime"])
		d.Path = string(m["path"])
		d.Comment = string(m["comment"])
	}
	return d
}

func (d Vm) All(trx trx.ITrx, offset int64, count int64) ([]Vm, error) {
	if count == -1 {
		objs, err := trx.GetObjList("Vm", []string{"*"}, map[string]string{})
		if err != nil {
			log.Println(err)
			return nil, err
		}
		entities := []Vm{}
		for id, m := range objs {
			if len(m) > 0 {
				d := Vm{}
				d.MachineId = id
				d.AppId = string(m["appId"])
				d.Runtime = string(m["runtime"])
				d.Path = string(m["path"])
				entities = append(entities, d)
			}
		}
		sort.Slice(entities, func(i, j int) bool {
			return entities[i].MachineId < entities[j].MachineId
		})
		return entities, nil
	} else {
		objs, err := trx.GetObjList("Vm", []string{"*"}, map[string]string{}, offset, count)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		entities := []Vm{}
		for id, m := range objs {
			if len(m) > 0 {
				d := Vm{}
				d.MachineId = id
				d.AppId = string(m["appId"])
				d.Runtime = string(m["runtime"])
				d.Path = string(m["path"])
				d.Comment = string(m["comment"])
				entities = append(entities, d)
			}
		}
		sort.Slice(entities, func(i, j int) bool {
			return entities[i].MachineId < entities[j].MachineId
		})
		return entities, nil
	}
}

func (d Vm) List(trx trx.ITrx, prefix string) ([]Vm, error) {
	list, err := trx.GetLinksList(prefix, -1, -1)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	for i := 0; i < len(list); i++ {
		list[i] = list[i][len(prefix):]
	}
	objs, err := trx.GetObjList("Vm", list, map[string]string{})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []Vm{}
	for id, m := range objs {
		if len(m) > 0 {
			d := Vm{}
			d.MachineId = id
			d.AppId = string(m["appId"])
			d.Runtime = string(m["runtime"])
			d.Path = string(m["path"])
			d.Comment = string(m["comment"])
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].MachineId < entities[j].MachineId
	})
	return entities, nil
}
