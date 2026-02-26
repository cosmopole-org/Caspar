package trx

import (
	"crypto/rsa"
	"kasper/src/abstract/models/update"
)

type IModel[T any] interface {
	Type() string
	Parse(ITrx) T
}

type ITrx interface {
	DelKey(key string)
	GetByPrefix(string) []string
	HasObj(typ string, key string) bool
	GetIndex(typ string, fromColumn string, toColumn string, fromColumnVal string) string
	PutIndex(typ string, fromColumn string, toColumn string, fromColumnVal string, toColumnVal []byte)
	DelIndex(typ string, fromColumn string, toColumn string, fromColumnVal string)
	HasIndex(typ string, fromColumn string, toColumn string, fromColumnVal string) bool
	GetColumn(typ string, objId string, columnName string) []byte
	GetLinksList(p string, offset int, count int, shouldBeGlobal ...bool) ([]string, error)
	SearchLinkValsList(typ string, fromColumn string, toColumn string, word string, filter map[string]string, offset int64, count int64) ([]string, error)
	SearchLinkKeysListByPrefix(p string, typ string, filter map[string]string, inArrFilter map[string][]string, offset int64, count int64, shouldBeGlobal ...bool) ([]string, error)
	GetObjList(typ string, objIds []string, query map[string]string, meta ...int64) (map[string]map[string][]byte, error)
	GetLink(key string) string
	PutLink(key string, value string)
	PutBytes(key string, value []byte)
	GetBytes(key string) []byte
	PutString(key string, value string)
	GetString(key string) string
	GetObj(typ string, key string) map[string][]byte
	PutObj(typ string, key string, keys map[string][]byte)
	PutJson(key string, path string, jsonObj any, merge bool) error
	DelJson(key string, path string)
	GetJson(key string, path string) (map[string]any, error)
	GetPriKey(string) *rsa.PrivateKey
	GetPubKey(string) *rsa.PublicKey
	Updates() []update.Update
	Commit()
	Discard()
}
