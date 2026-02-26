package main

import (
	"fmt"
	"io/ioutil"
	"kasper/src/drivers/network/chain/crypto/keys"
	"log"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
)

func main() {
	if err := generate(); err != nil {
		log.Println(err)
	}
}

func HomeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}

func DefaultDataDir() string {
	home := HomeDir()
	if home != "" {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, ".Babble")
		} else if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "Babble")
		} else {
			return filepath.Join(home, ".babble")
		}
	}
	return ""
}

var (
	privKeyFile string = fmt.Sprintf("%s/priv_key", DefaultDataDir())
	pubKeyFile  string = fmt.Sprintf("%s/key.pub", DefaultDataDir())
)

func generate() error {
	if _, err := os.Stat(privKeyFile); err == nil {
		return fmt.Errorf("a key already lives under: %s", path.Dir(privKeyFile))
	}

	key, err := keys.GenerateECDSAKey()
	if err != nil {
		return fmt.Errorf("error generating ECDSA key")
	}

	if err := os.MkdirAll(path.Dir(privKeyFile), 0700); err != nil {
		return fmt.Errorf("writing private key: %s", err)
	}

	jsonKey := keys.NewSimpleKeyfile(privKeyFile)

	if err := jsonKey.WriteKey(key); err != nil {
		return fmt.Errorf("writing private key: %s", err)
	}

	fmt.Printf("Your private key has been saved to: %s\n", privKeyFile)

	if err := os.MkdirAll(path.Dir(pubKeyFile), 0700); err != nil {
		return fmt.Errorf("writing public key: %s", err)
	}

	pub := keys.PublicKeyHex(&key.PublicKey)

	if err := ioutil.WriteFile(pubKeyFile, []byte(pub), 0600); err != nil {
		return fmt.Errorf("writing public key: %s", err)
	}

	fmt.Printf("Your public key has been saved to: %s\n", pubKeyFile)

	return nil
}
