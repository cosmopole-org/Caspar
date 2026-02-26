package module_trx

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"kasper/src/abstract/adapters/storage"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/trx"
	"kasper/src/abstract/models/update"
	"kasper/src/shell/utils/crypto"
	"log"
	"slices"
	"sort"
	"strings"

	"github.com/dgraph-io/badger"
)

type TrxWrapper struct {
	core    core.ICore
	dbTrx   *badger.Txn
	Changes []update.Update
}

func NewTrx(core core.ICore, storage storage.IStorage, readonly bool) trx.ITrx {
	tw := &TrxWrapper{core: core, Changes: []update.Update{}}
	tw.dbTrx = storage.KvDb().NewTransaction(!readonly)
	return tw
}

func (tw *TrxWrapper) GetColumn(typ string, objId string, columnName string) []byte {
	item, e := tw.dbTrx.Get([]byte("obj::" + typ + "::" + objId + "::" + columnName))
	if e != nil {
		return []byte{}
	} else {
		res, _ := item.ValueCopy(nil)
		return res
	}
}

func (tw *TrxWrapper) Discard() {
	tw.dbTrx.Discard()
}

func (tw *TrxWrapper) Commit() {
	e := tw.dbTrx.Commit()
	if e != nil {
		log.Println("Error on committing:", e)
		log.Println("retrying commit in safe context")
		err := tw.core.Tools().Storage().KvDb().Update(func(txn *badger.Txn) error {
			for _, change := range tw.Changes {
				if change.Typ == "put" {
					if err := txn.Set([]byte(change.Key), change.Val); err != nil {
						return err
					}
				} else if change.Typ == "del" {
					if err := txn.Delete([]byte(change.Key)); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			log.Println("Error on safe context committing:", err)
		}
	}
}

func (tw *TrxWrapper) DelKey(key string) {
	tw.dbTrx.Delete([]byte(key))
	tw.Changes = append(tw.Changes, update.Update{Typ: "del", Key: string([]byte(key))})
}

func (tw *TrxWrapper) HasObj(typ string, key string) bool {
	_, e := tw.dbTrx.Get([]byte("obj::" + typ + "::" + key + "::|"))
	return e == nil
}

func (tw *TrxWrapper) GetIndex(typ string, fromColumn string, toColumn string, fromColumnVal string) string {
	item, e := tw.dbTrx.Get([]byte("index::" + typ + "::" + fromColumn + "::" + toColumn + "::" + fromColumnVal))
	if e != nil {
		return ""
	} else {
		value, _ := item.ValueCopy(nil)
		return string(value)
	}
}

func (tw *TrxWrapper) PutIndex(typ string, fromColumn string, toColumn string, fromColumnVal string, toColumnVal []byte) {
	tw.dbTrx.Set([]byte("index::"+typ+"::"+fromColumn+"::"+toColumn+"::"+fromColumnVal), toColumnVal)
	tw.Changes = append(tw.Changes, update.Update{Typ: "put", Key: "index::" + string([]byte(typ)) + "::" + string([]byte(fromColumn)) + "::" + string([]byte(toColumn)) + "::" + string([]byte(fromColumnVal)), Val: []byte(string(toColumnVal))})
}

func (tw *TrxWrapper) DelIndex(typ string, fromColumn string, toColumn string, fromColumnVal string) {
	tw.DelKey("index::" + typ + "::" + fromColumn + "::" + toColumn + "::" + fromColumnVal)
}

func (tw *TrxWrapper) HasIndex(typ string, fromColumn string, toColumn string, fromColumnVal string) bool {
	_, e := tw.dbTrx.Get([]byte("index::" + typ + "::" + fromColumn + "::" + toColumn + "::" + fromColumnVal))
	return e == nil
}

func (tw *TrxWrapper) GetLink(key string) string {
	item, e := tw.dbTrx.Get([]byte("link::" + key))
	if e != nil {
		return ""
	} else {
		value, _ := item.ValueCopy(nil)
		return string(value)
	}
}

func (tw *TrxWrapper) PutLink(key string, value string) {
	tw.dbTrx.Set([]byte("link::"+key), []byte(value))
	tw.Changes = append(tw.Changes, update.Update{Typ: "put", Key: "link::" + string([]byte(key)), Val: []byte(value)})
}

func (tw *TrxWrapper) PutBytes(key string, value []byte) {
	tw.dbTrx.Set([]byte(key), value)
	tw.Changes = append(tw.Changes, update.Update{Typ: "put", Key: string([]byte(key)), Val: []byte(string(value))})
}

func (tw *TrxWrapper) GetBytes(key string) []byte {
	item, e := tw.dbTrx.Get([]byte(key))
	if e == nil {
		value, _ := item.ValueCopy(nil)
		return value
	}
	return []byte{}
}

func (tw *TrxWrapper) PutString(key string, value string) {
	tw.dbTrx.Set([]byte(key), []byte(value))
	tw.Changes = append(tw.Changes, update.Update{Typ: "put", Key: string([]byte(key)), Val: []byte(value)})
}

func (tw *TrxWrapper) GetString(key string) string {
	item, e := tw.dbTrx.Get([]byte(key))
	if e != nil {
		return ""
	} else {
		value, _ := item.ValueCopy(nil)
		return string(value)
	}
}

func (tw *TrxWrapper) GetByPrefix(p string) []string {
	prefix := []byte(p)
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = true
	opts.Prefix = prefix
	it := tw.dbTrx.NewIterator(opts)
	defer it.Close()
	m := []string{}
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		item := it.Item()
		itemKey := item.Key()
		m = append(m, string(itemKey))
	}
	return m
}

func (tw *TrxWrapper) GetObj(typ string, key string) map[string][]byte {
	prefix := []byte("obj::" + typ + "::" + key + "::")
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = true
	opts.Prefix = prefix
	it := tw.dbTrx.NewIterator(opts)
	defer it.Close()
	m := map[string][]byte{}
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		item := it.Item()
		itemKey := item.Key()
		itemVal, _ := item.ValueCopy(nil)
		m[string(itemKey[len(prefix):])] = itemVal
	}
	return m
}

func (tw *TrxWrapper) PutObj(typ string, key string, keys map[string][]byte) {
	prefix := "obj::" + typ + "::" + key + "::"
	keys["|"] = []byte{0x01}
	for k, v := range keys {
		tw.dbTrx.Set([]byte(prefix+k), v)
		tw.Changes = append(tw.Changes, update.Update{Typ: "put", Key: string([]byte(prefix + k)), Val: []byte(string(v))})
	}
}

func mergeObjects(dst map[string]any, src map[string]any) map[string]any {
	for k, v := range src {
		if mSrc, ok := v.(map[string]any); ok {
			if dst[k] == nil {
				dst[k] = v
			} else if mDst, ok := dst[k].(map[string]any); ok {
				dst[k] = mergeObjects(mDst, mSrc)
			} else {
				dst[k] = v
			}
		} else {
			dst[k] = v
		}
	}
	return dst
}

func (tw *TrxWrapper) indexJson(key string, path string, obj map[string]any, merge bool) {
	old := map[string]any{}
	if merge {
		var e error
		old, e = tw.GetJson(key, path)
		if e != nil {
			old = map[string]any{}
			log.Println(e)
		}
	}
	keys := make([]string, 0, len(obj))
	for k, _ := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	old = mergeObjects(old, obj)
	b, _ := json.Marshal(old)
	tw.PutBytes("json::"+key+"::"+path, b)
	for _, k := range keys {
		v := obj[k]
		if v != nil {
			if m, ok := v.(map[string]any); ok {
				tw.indexJson(key, path+"."+k, m, merge)
			} else {
				b, _ := json.Marshal(v)
				tw.PutBytes("json::"+key+"::"+path+"."+k, b)
			}
		}
	}
}

func (tw *TrxWrapper) PutJson(key string, path string, jsonObj any, merge bool) error {
	b, e := json.Marshal(jsonObj)
	if e != nil {
		return e
	}
	m := map[string]any{}
	e = json.Unmarshal(b, &m)
	if e != nil {
		return e
	}
	tw.indexJson(key, path, m, merge)
	return nil
}

func (tw *TrxWrapper) DelJson(key string, path string) {
	tw.DelKey("json::" + key + "::" + path)
}

func (tw *TrxWrapper) GetJson(key string, path string) (map[string]any, error) {
	b := tw.GetBytes("json::" + key + "::" + path)
	m := map[string]any{}
	if len(b) == 0 {
		return m, errors.New("json path not found")
	}
	e := json.Unmarshal(b, &m)
	if e != nil {
		return m, e
	}
	return m, nil
}

func (tw *TrxWrapper) GetObjList(typ string, objIds []string, queryMap map[string]string, meta ...int64) (map[string]map[string][]byte, error) {
	if (len(objIds) == 1) && (objIds[0] == "*") {
		objs := map[string]map[string][]byte{}
		prefix := []byte("obj::" + typ + "::")
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		opts.Prefix = prefix
		it := tw.dbTrx.NewIterator(opts)
		defer it.Close()
		if len(meta) == 0 {
			temp := map[string][]byte{}
			tempId := ""
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				item := it.Item()
				itemKey := item.Key()
				id := strings.Split(string(itemKey[len(prefix):]), "::")[0]
				itemVal, _ := item.ValueCopy(nil)
				if tempId != id {
					matched := true
					if len(queryMap) > 0 {
						for k, v := range queryMap {
							if v != string(temp[k]) {
								matched = false
							}
						}
					}
					if _, ok := temp["|"]; ok && matched && (tempId != "") {
						objs[tempId] = temp
					}
					temp = map[string][]byte{}
					tempId = id
				}
				temp[string(itemKey)[len(string(prefix))+len(id)+len("::"):]] = itemVal
			}
			matched := true
			if len(queryMap) > 0 {
				for k, v := range queryMap {
					if (len(temp[k]) == 0) || (v != string(temp[k])) {
						matched = false
					}
				}
			}
			if _, ok := temp["|"]; ok && matched && (tempId != "") {
				objs[tempId] = temp
			}
		} else if len(meta) == 1 {
			index := int64(0)
			offset := meta[0]
			temp := map[string][]byte{}
			tempId := ""
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				item := it.Item()
				itemKey := item.Key()
				id := strings.Split(string(itemKey[len(prefix):]), "::")[0]
				itemVal, _ := item.ValueCopy(nil)
				if tempId != id {
					matched := true
					if len(queryMap) > 0 {
						for k, v := range queryMap {
							if v != string(temp[k]) {
								matched = false
							}
						}
					}
					if _, ok := temp["|"]; ok && matched && (tempId != "") {
						if index < offset {
							index++
							temp = map[string][]byte{}
							tempId = id
							temp[string(itemKey)[len(string(prefix))+len(id)+len("::"):]] = itemVal
							continue
						}
						index++
						objs[tempId] = temp
					}
					temp = map[string][]byte{}
					tempId = id
				}
				temp[string(itemKey)[len(string(prefix))+len(id)+len("::"):]] = itemVal
			}
			matched := true
			if len(queryMap) > 0 {
				for k, v := range queryMap {
					if (len(temp[k]) == 0) || (v != string(temp[k])) {
						matched = false
					}
				}
			}
			if _, ok := temp["|"]; ok && matched && (tempId != "") {
				if index >= offset {
					objs[tempId] = temp
				}
			}
		} else if len(meta) == 2 {
			index := int64(0)
			offset := meta[0]
			count := meta[1]
			temp := map[string][]byte{}
			tempId := ""
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				item := it.Item()
				itemKey := item.Key()
				id := strings.Split(string(itemKey[len(prefix):]), "::")[0]
				itemVal, _ := item.ValueCopy(nil)
				log.Println("parsing field...", tempId, id, itemKey)
				if tempId != id {
					log.Println("id before", id, tempId, index, offset, count)
					matched := true
					if len(queryMap) > 0 {
						for k, v := range queryMap {
							if v != string(temp[k]) {
								log.Println(k, v, string(temp[k]))
								matched = false
							}
						}
					}
					log.Println("match state:", matched)
					if _, ok := temp["|"]; ok && matched && (tempId != "") {
						log.Println("id", id, tempId, index, offset, count)
						if index < offset {
							index++
							temp = map[string][]byte{}
							tempId = id
							temp[string(itemKey)[len(string(prefix))+len(id)+len("::"):]] = itemVal
							continue
						}
						if index >= (offset + count) {
							break
						}
						index++
						log.Println("id after", id, tempId, index, offset, count)
						objs[tempId] = temp
					}
					temp = map[string][]byte{}
					tempId = id
				}
				temp[string(itemKey)[len(string(prefix))+len(id)+len("::"):]] = itemVal
			}
			matched := true
			if len(queryMap) > 0 {
				for k, v := range queryMap {
					if (len(temp[k]) == 0) || (v != string(temp[k])) {
						matched = false
					}
				}
			}
			if _, ok := temp["|"]; ok && matched && (tempId != "") {
				if index >= offset && index < (offset+count) {
					objs[tempId] = temp
				}
			}
		}
		return objs, nil
	} else {
		m := map[string]map[string][]byte{}
		for _, id := range objIds {
			m[id] = tw.GetObj(typ, id)
		}
		return m, nil
	}
}

func (tw *TrxWrapper) GetLinksList(p string, offset int, count int, shouldBeGlobal ...bool) ([]string, error) {
	if len(shouldBeGlobal) > 0 && shouldBeGlobal[0] {
		prefix := []byte("link::" + p)
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		opts.Prefix = prefix
		it := tw.dbTrx.NewIterator(opts)
		defer it.Close()
		m := []string{}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			itemKey := item.Key()
			if strings.HasSuffix(string(itemKey), "@global") {
				m = append(m, string(itemKey)[len("link::"):])
			}
		}
		return m, nil
	} else {
		prefix := []byte("link::" + p)
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		opts.Prefix = prefix
		it := tw.dbTrx.NewIterator(opts)
		defer it.Close()
		m := []string{}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			itemKey := item.Key()
			m = append(m, string(itemKey)[len("link::"):])
		}
		return m, nil
	}
}

func (tw *TrxWrapper) SearchLinkValsList(typ string, fromColumn string, toColumn string, word string, filter map[string]string, offset int64, count int64) ([]string, error) {
	prefix := []byte("index::" + typ + "::" + fromColumn + "::" + toColumn + "::")
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = true
	opts.Prefix = prefix
	it := tw.dbTrx.NewIterator(opts)
	defer it.Close()
	m := []string{}
	counter := int64(0)
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		item := it.Item()
		itemKey := item.Key()
		if strings.Contains(string(itemKey[len(prefix):]), word) {
			itemVal, _ := item.ValueCopy(nil)
			matched := true
			if len(filter) > 0 {
				for k, v := range filter {
					if string(tw.GetColumn(typ, string(itemVal), k)) != v {
						matched = false
						break
					}
				}
			}
			if matched {
				if counter < offset {
					counter++
					continue
				}
				if counter >= (offset + count) {
					break
				}
				m = append(m, string(itemVal))
				counter++
			}
		}
	}
	return m, nil
}

func (tw *TrxWrapper) SearchLinkKeysListByPrefix(p string, typ string, filter map[string]string, inArrFilter map[string][]string, offset int64, count int64, shouldBeGlobal ...bool) ([]string, error) {
	prefix := []byte("link::" + p)
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = true
	opts.Prefix = prefix
	it := tw.dbTrx.NewIterator(opts)
	defer it.Close()
	m := []string{}
	counter := int64(0)
	if len(shouldBeGlobal) > 0 && shouldBeGlobal[0] {
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			itemKey := item.Key()
			objId := string(itemKey)[len(string(prefix)):]
			matched := true
			if strings.HasSuffix(string(itemKey), "@global") {
				if len(filter) > 0 {
					for k, v := range filter {
						if string(tw.GetColumn(typ, objId, k)) != v {
							matched = false
							break
						}
					}
				}
				if len(inArrFilter) > 0 {
					for k, v := range inArrFilter {
						if !slices.Contains(v, string(tw.GetColumn(typ, objId, k))) {
							matched = false
							break
						}
					}
				}
				if matched {
					if counter < offset {
						counter++
						continue
					}
					if counter >= (offset + count) {
						break
					}
					m = append(m, objId)
					counter++
				}
			}
		}
	} else {
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			itemKey := item.Key()
			objId := string(itemKey)[len(string(prefix)):]
			matched := true
			if len(filter) > 0 {
				for k, v := range filter {
					if string(tw.GetColumn(typ, objId, k)) != v {
						matched = false
						break
					}
				}
			}
			if len(inArrFilter) > 0 {
				for k, v := range inArrFilter {
					if !slices.Contains(v, string(tw.GetColumn(typ, objId, k))) {
						matched = false
						break
					}
				}
			}
			if matched {
				if counter < offset {
					counter++
					continue
				}
				if counter >= (offset + count) {
					break
				}
				m = append(m, objId)
				counter++
			}
		}
	}
	return m, nil
}

func (tw *TrxWrapper) GetPriKey(key string) *rsa.PrivateKey {
	res := tw.GetString("obj::User::" + key + "::privateKey")
	if res == "" {
		return nil
	} else {
		return crypto.ParsePrivateKey([]byte(res))
	}
}

func (tw *TrxWrapper) GetPubKey(key string) *rsa.PublicKey {
	res := tw.GetString("obj::User::" + key + "::publicKey")
	if res == "" {
		return nil
	} else {
		return crypto.ParsePublicKey([]byte(res))
	}
}

func (tw *TrxWrapper) Updates() []update.Update {
	return tw.Changes
}
