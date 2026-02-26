package model

import (
	"encoding/binary"
	"kasper/src/abstract/models/trx"
	"log"
	"sort"
)

type User struct {
	Id        string `json:"id"`
	Typ       string `json:"type"`
	Username  string `json:"username"`
	PublicKey string `json:"publicKey"`
	Balance   int64  `json:"balance"`
}

func (d User) Type() string {
	return "User"
}

func (d User) Push(trx trx.ITrx) {
	bal := make([]byte, 8)
	binary.LittleEndian.PutUint64(bal, uint64(d.Balance))
	trx.PutObj(d.Type(), d.Id, map[string][]byte{
		"type":      []byte(d.Typ),
		"username":  []byte(d.Username),
		"publicKey": []byte(d.PublicKey),
		"balance":   bal,
	})
	trx.PutIndex("User", "username", "id", d.Username, []byte(d.Id))
}

func (d User) Delete(trx trx.ITrx) {
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::|")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::id")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::type")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::username")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::publicKey")
	trx.DelKey("obj::" + d.Type() + "::" + d.Id + "::balance")
	trx.DelJson("UserMeta::"+d.Id, "metadata")
}

func (d User) Pull(trx trx.ITrx) User {
	m := trx.GetObj(d.Type(), d.Id)
	if len(m) > 0 {
		d.Typ = string(m["type"])
		d.Username = string(m["username"])
		d.PublicKey = string(m["publicKey"])
		d.Balance = int64(binary.LittleEndian.Uint64(m["balance"]))
	}
	return d
}

func (d User) List(trx trx.ITrx, prefix string, query map[string]string) ([]User, error) {
	list, err := trx.GetLinksList(prefix, -1, -1)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	for i := 0; i < len(list); i++ {
		list[i] = list[i][len(prefix):]
	}
	objs, err := trx.GetObjList("User", list, query)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []User{}
	for id, m := range objs {
		if len(m) > 0 {
			d := User{}
			d.Id = id
			d.Typ = string(m["type"])
			d.Username = string(m["username"])
			d.PublicKey = string(m["publicKey"])
			d.Balance = int64(binary.LittleEndian.Uint64(m["balance"]))
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Id < entities[j].Id
	})
	return entities, nil
}

func (d User) All(trx trx.ITrx, offset int64, count int64, query map[string]string) ([]User, error) {
	objs, err := trx.GetObjList("User", []string{"*"}, query, offset, count)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []User{}
	for id, m := range objs {
		if len(m) > 0 {
			d := User{}
			d.Id = id
			d.Typ = string(m["type"])
			d.Username = string(m["username"])
			d.PublicKey = string(m["publicKey"])
			d.Balance = int64(binary.LittleEndian.Uint64(m["balance"]))
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Id < entities[j].Id
	})
	return entities, nil
}

func (d User) Search(trx trx.ITrx, offset int64, count int64, fromColumn string, word string, filter map[string]string) ([]User, error) {
	links, err := trx.SearchLinkValsList("User", fromColumn, "id", word, filter, offset, count)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	objs, err := trx.GetObjList("User", links, map[string]string{})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	entities := []User{}
	for id, m := range objs {
		if len(m) > 0 {
			d := User{}
			d.Id = id
			d.Typ = string(m["type"])
			d.Username = string(m["username"])
			d.PublicKey = string(m["publicKey"])
			d.Balance = int64(binary.LittleEndian.Uint64(m["balance"]))
			entities = append(entities, d)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Id < entities[j].Id
	})
	return entities, nil
}
