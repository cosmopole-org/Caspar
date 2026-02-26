package model

import (
	"bytes"
	"encoding/binary"
	"kasper/src/abstract/models/trx"
	"log"
	"sort"
)

type Point struct {
	Id          string `json:"id"`
	Tag         string `json:"tag"`
	ParentId    string `json:"parentId"`
	PersHist    bool   `json:"persHist"`
	IsPublic    bool   `json:"isPublic"`
	MemberCount int32  `json:"memberCount"`
	SignalCount int64  `json:"signalCount"`
}

func (d Point) Type() string {
	return "Point"
}

func (d Point) Push(trx trx.ITrx) {
	b := byte(0x00)
	if d.IsPublic {
		b = byte(0x01)
	}
	b2 := byte(0x00)
	if d.PersHist {
		b2 = byte(0x01)
	}
	b3 := make([]byte, 4)
	if d.MemberCount < 1 {
		d.MemberCount = 1
	}
	b4 := make([]byte, 8)
	binary.LittleEndian.PutUint32(b3, uint32(d.MemberCount))
	binary.LittleEndian.PutUint64(b4, uint64(d.SignalCount))
	trx.PutObj(d.Type(), d.Id, map[string][]byte{
		"tag":         []byte(d.Tag),
		"parentId":    []byte(d.ParentId),
		"isPublic":    {b},
		"persHist":    {b2},
		"memberCount": b3,
		"signalCount": b4,
	})
}

func (d Point) Delete(trx trx.ITrx) {
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::|")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::id")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::tag")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::parentId")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::isPublic")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::persHist")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::memberCount")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::signalCount")
	trx.DelJson("PointMeta::"+d.Id, "metadata")
}

func (d Point) Pull(trx trx.ITrx) Point {
	m := trx.GetObj(d.Type(), d.Id)
	if len(m) > 0 {
		d.Tag = string(m["tag"])
		d.ParentId = string(m["parentId"])
		d.MemberCount = int32(binary.LittleEndian.Uint32(m["memberCount"]))
		d.SignalCount = int64(binary.LittleEndian.Uint64(m["signalCount"]))
		d.IsPublic = bytes.Equal(m["isPublic"], []byte{0x01})
		d.PersHist = bytes.Equal(m["persHist"], []byte{0x01})
	}
	return d
}

func (d Point) List(trx trx.ITrx, prefix string, global bool, filter map[string]string, inArrFilter map[string][]string, positional ...int64) ([]Point, error) {
	offset := int64(0)
	count := int64(50)
	entities := []Point{}
	if len(positional) == 1 {
		offset = positional[0]
	}
	if len(positional) == 2 {
		count = positional[1]
	}
	list, err := trx.SearchLinkKeysListByPrefix(prefix, "Point", filter, inArrFilter, offset, count, global)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	objs, err := trx.GetObjList("Point", list, map[string]string{})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	for id, m := range objs {
		if len(m) > 0 {
			d := Point{}
			d.Id = id
			d.Tag = string(m["tag"])
			d.ParentId = string(m["parentId"])
			d.MemberCount = int32(binary.LittleEndian.Uint32(m["memberCount"]))
			d.SignalCount = int64(binary.LittleEndian.Uint64(m["signalCount"]))
			d.IsPublic = bytes.Equal(m["isPublic"], []byte{0x01})
			d.PersHist = bytes.Equal(m["persHist"], []byte{0x01})
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Id < entities[j].Id
	})
	return entities, nil
}

func (d Point) All(trx trx.ITrx, offset int64, count int64, query map[string]string) ([]Point, error) {
	objs, err := trx.GetObjList("Point", []string{"*"}, query, offset, count)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []Point{}
	for id, m := range objs {
		if len(m) > 0 {
			d := Point{}
			d.Id = id
			d.Tag = string(m["tag"])
			d.ParentId = string(m["parentId"])
			d.MemberCount = int32(binary.LittleEndian.Uint32(m["memberCount"]))
			d.SignalCount = int64(binary.LittleEndian.Uint64(m["signalCount"]))
			d.IsPublic = bytes.Equal(m["isPublic"], []byte{0x01})
			d.PersHist = bytes.Equal(m["persHist"], []byte{0x01})
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Id < entities[j].Id
	})
	return entities, nil
}

func (d Point) Search(trx trx.ITrx, offset int64, count int64, word string, filter map[string]string) ([]Point, error) {
	links, err := trx.SearchLinkValsList("Point", "title", "id", word, filter, offset, count)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	objs, err := trx.GetObjList("Point", links, map[string]string{})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []Point{}
	for id, m := range objs {
		if len(m) > 0 {
			d := Point{}
			d.Id = id
			d.Tag = string(m["tag"])
			d.ParentId = string(m["parentId"])
			d.MemberCount = int32(binary.LittleEndian.Uint32(m["memberCount"]))
			d.SignalCount = int64(binary.LittleEndian.Uint64(m["signalCount"]))
			d.IsPublic = bytes.Equal(m["isPublic"], []byte{0x01})
			d.PersHist = bytes.Equal(m["persHist"], []byte{0x01})
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Id < entities[j].Id
	})
	return entities, nil
}
