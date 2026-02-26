package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/google/uuid"
)

func SecureUniqueString() string {
	return uuid.New().String() + "-" + uuid.New().String()
}

func SecureUniqueId(fed string) string {
	return uuid.New().String() + "@" + fed
}

func SecureKeyPairs(savePath string) ([]byte, []byte) {

	if savePath != "" {
		os.MkdirAll(savePath, os.ModePerm)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	publicKey := &privateKey.PublicKey

	privBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		panic(err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	pubBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		panic(err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	fmt.Println("Private Key PEM:\n", string(privPEM))
	fmt.Println("Public Key PEM:\n", string(pubPEM))

	if savePath != "" {
		err = os.WriteFile(savePath+"/public.pem", pubPEM, 0644)
		if err != nil {
			panic(err)
		}
		err = os.WriteFile(savePath+"/private.pem", privPEM, 0644)
		if err != nil {
			panic(err)
		}
	}

	return privPEM, pubPEM
}

func ParsePrivateKey(data []byte) *rsa.PrivateKey {
	parsedPrivBlock, _ := pem.Decode(data)
	parsedPrivKeyIface, err := x509.ParsePKCS8PrivateKey(parsedPrivBlock.Bytes)
	if err != nil {
		panic(err)
	}
	parsedPrivKey := parsedPrivKeyIface.(*rsa.PrivateKey)
	return parsedPrivKey
}

func ParsePublicKey(data []byte) *rsa.PublicKey {
	parsedPubBlock, _ := pem.Decode(data)
	parsedPubKeyIface, err := x509.ParsePKIXPublicKey(parsedPubBlock.Bytes)
	if err != nil {
		panic(err)
	}
	parsedPubKey := parsedPubKeyIface.(*rsa.PublicKey)
	return parsedPubKey
}
