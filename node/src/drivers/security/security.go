package security

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"kasper/src/abstract/adapters/security"
	"kasper/src/abstract/adapters/signaler"
	"kasper/src/abstract/adapters/storage"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/trx"
	cryp "kasper/src/shell/utils/crypto"
	"kasper/src/shell/utils/vaidate"
	"log"
	"os"
)

type Security struct {
	app         core.ICore
	storage     storage.IStorage
	signaler    signaler.ISignaler
	storageRoot string
	keys        map[string][][]byte
}

type AuthHolder struct {
	Token string `json:"token"`
}

type LastPos struct {
	UserId   int64
	UserType int32
	SpaceId  int64
	TopicId  int64
	WorkerId int64
}

type Location struct {
	PointId string
}

const keysFolderName = "keys"

func (sm *Security) LoadKeys() {
	files, err := os.ReadDir(sm.storageRoot + "/keys")
	if err != nil {
		log.Println(err)
	}
	for _, file := range files {
		if file.IsDir() {
			priKey, err1 := os.ReadFile(sm.storageRoot + "/" + keysFolderName + "/" + file.Name() + "/private.pem")
			if err1 != nil {
				log.Println(err1)
				continue
			}
			pubKey, err2 := os.ReadFile(sm.storageRoot + "/" + keysFolderName + "/" + file.Name() + "/public.pem")
			if err2 != nil {
				log.Println(err2)
				continue
			}
			sm.keys[file.Name()] = [][]byte{priKey, pubKey}
		}
	}
	if sm.FetchKeyPair("server_key") == nil {
		sm.GenerateSecureKeyPair("server_key")
	}
}

func (sm *Security) GenerateSecureKeyPair(tag string) {
	var priKey, pubKey = cryp.SecureKeyPairs(sm.storageRoot + "/" + keysFolderName + "/" + tag)
	sm.keys[tag] = [][]byte{priKey, pubKey}
}

func (sm *Security) FetchKeyPair(tag string) [][]byte {
	return sm.keys[tag]
}

func (sm *Security) Encrypt(tag string, plainText string) string {
	publicKeyPEM := sm.keys[tag][1]
	publicKeyBlock, _ := pem.Decode(publicKeyPEM)
	publicKey, err := x509.ParsePKIXPublicKey(publicKeyBlock.Bytes)
	if err != nil {
		log.Println(err)
		return ""
	}
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, publicKey.(*rsa.PublicKey), []byte(plainText))
	if err != nil {
		log.Println(err)
		return ""
	}
	return fmt.Sprintf("%x", ciphertext)
}

func (sm *Security) Decrypt(tag string, cipherText string) string {
	privateKeyPEM := sm.keys[tag][0]
	privateKeyBlock, _ := pem.Decode(privateKeyPEM)
	privateKey, err := x509.ParsePKCS1PrivateKey(privateKeyBlock.Bytes)
	if err != nil {
		log.Println(err)
		return ""
	}
	plaintext, err := rsa.DecryptPKCS1v15(rand.Reader, privateKey, []byte(cipherText))
	if err != nil {
		log.Println(err)
		return ""
	}
	return string(plaintext)
}

func (sm *Security) AuthWithSignature(userId string, packet []byte, signatureBase64 string) (bool, string, bool) {
	var publicKey *rsa.PublicKey
	sm.app.ModifyState(true, func(trx trx.ITrx) error {
		publicKey = trx.GetPubKey(userId)
		return nil
	})
	if publicKey == nil {
		return false, "", false
	}
	signature, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		log.Println(err)
		return false, "", false
	}
	hash := sha256.Sum256(packet)
	err = rsa.VerifyPSS(publicKey, crypto.SHA256, hash[:], signature, &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
	})
	if err != nil {
		fmt.Println("Verification failed:", err)
		log.Println(err)
		return false, "", false
	}
	fmt.Println("Signature verified successfully!")
	var userType = ""
	var isGod = false
	sm.app.ModifyState(true, func(trx trx.ITrx) error {
		userType = string(trx.GetColumn("User", userId, "type"))
		isGod = (trx.GetString("god::"+userId) == "true")
		return nil
	})
	return true, userType, isGod
}

func (sm *Security) HasAccessToPoint(userId string, pointId string) bool {
	if pointId == "" {
		return false
	}
	found := false
	sm.app.ModifyState(true, func(trx trx.ITrx) error {
		if trx.GetLink("memberof::"+userId+"::"+pointId) == "true" {
			found = true
		}
		return nil
	})
	return found
}

func New(core core.ICore, storageRoot string, storage storage.IStorage, signaler signaler.ISignaler) security.ISecurity {
	vaidate.LoadValidationSystem()
	s := &Security{
		app:         core,
		storage:     storage,
		signaler:    signaler,
		storageRoot: storageRoot,
		keys:        make(map[string][][]byte),
	}
	s.LoadKeys()
	return s
}
