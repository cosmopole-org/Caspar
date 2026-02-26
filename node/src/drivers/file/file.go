package tool_file

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
)

type File struct {
}

func (g *File) CheckFileFromStorage(storageRoot string, topicId string, key string) bool {
	if _, err := os.Stat(fmt.Sprintf("%s/files/%s/%s", storageRoot, topicId, key)); errors.Is(err, os.ErrNotExist) {
		return false
	} else {
		return true
	}
}

func (g *File) SaveFileToStorage(storageRoot string, fh *multipart.FileHeader, topicId string, key string) error {
	var dirPath = fmt.Sprintf("%s/files/%s", storageRoot, topicId)
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return err
	}
	log.Println("trying to start file operation...")
	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer func(f multipart.File) {
		err := f.Close()
		if err != nil {
			log.Println(err)
		}
	}(f)
	log.Println("opened received file.")
	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, f); err != nil {
		return err
	}
	dest, err := os.OpenFile(fmt.Sprintf("%s/%s", dirPath, key), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer func(dest *os.File) {
		err := dest.Close()
		if err != nil {
			log.Println(err)
		}
	}(dest)
	log.Println("opened created file.")
	if _, err = dest.Write(buf.Bytes()); err != nil {
		return err
	}
	return nil
}

func (g *File) SaveDataToStorage(storageRoot string, data []byte, topicId string, key string, flag ...bool) error {
	var dirPath = fmt.Sprintf("%s/files/%s", storageRoot, topicId)
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return err
	}
	var flags = 0
	if len(flag) > 0 && flag[0] {
		flags = os.O_WRONLY | os.O_CREATE
	} else {
		flags = os.O_APPEND | os.O_WRONLY | os.O_CREATE
	}
	dest, err := os.OpenFile(fmt.Sprintf("%s/%s", dirPath, key), flags, 0600)
	if err != nil {
		return err
	}
	defer func(dest *os.File) {
		err := dest.Close()
		if err != nil {
			log.Println(err)
		}
	}(dest)
	log.Println("opened created file.")
	if _, err = dest.Write(data); err != nil {
		return err
	}
	return nil
}

func (g *File) SaveTarFileItemToStorage(storageRoot string, fh *tar.Reader, topicId string, key string) error {
	var dirPath = fmt.Sprintf("%s/files/%s", storageRoot, topicId)
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return err
	}
	log.Println("opened received file.")
	dest, err := os.OpenFile(fmt.Sprintf("%s/%s", dirPath, key), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer func(dest *os.File) {
		err := dest.Close()
		if err != nil {
			log.Println(err)
		}
	}(dest)
	if _, err := io.Copy(dest, fh); err != nil {
		return err
	}
	return nil
}

func (g *File) ReadFileByPath(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return []byte{}, err
	}
	return content, nil
}

func (g *File) ReadFileFromStorage(storageRoot string, topicId string, key string) ([]byte, error) {
	content, err := os.ReadFile(fmt.Sprintf("%s/files/%s/%s", storageRoot, topicId, key))
	if err != nil {
		return []byte{}, err
	}
	return content, nil
}

func (g *File) CheckFileFromGlobalStorage(storageRoot string, key string) bool {
	if _, err := os.Stat(fmt.Sprintf("%s/%s", storageRoot, key)); errors.Is(err, os.ErrNotExist) {
		return false
	} else {
		return true
	}
}

func (g *File) ReadFileFromGlobalStorage(storageRoot string, key string) (string, error) {
	content, err := os.ReadFile(fmt.Sprintf("%s/%s", storageRoot, key))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (g *File) SaveFileToGlobalStorage(storageRoot string, fh *multipart.FileHeader, key string, overwrite bool) error {
	var dirPath = storageRoot
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return err
	}
	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer func(f multipart.File) {
		err := f.Close()
		if err != nil {
			log.Println(err)
		}
	}(f)
	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, f); err != nil {
		return err
	}
	if g.CheckFileFromGlobalStorage(storageRoot, key) {
		err := os.Remove(fmt.Sprintf("%s/%s", dirPath, key))
		if err != nil {
			log.Println(err)
		}
	}
	var flags = 0
	if overwrite {
		flags = os.O_WRONLY | os.O_CREATE
	} else {
		flags = os.O_APPEND | os.O_WRONLY | os.O_CREATE
	}
	dest, err := os.OpenFile(fmt.Sprintf("%s/%s", dirPath, key), flags, 0600)
	if err != nil {
		return err
	}
	defer func(dest *os.File) {
		err := dest.Close()
		if err != nil {
			log.Println(err)
		}
	}(dest)
	if _, err = dest.Write(buf.Bytes()); err != nil {
		return err
	}
	return nil
}

func (g *File) DeleteFileFromGlobalStorage(storageRoot string, key string, overwrite bool) error {
	filePath := fmt.Sprintf("%s/%s", storageRoot, key)
	_, err := os.Stat(filePath)
	if err != nil {
		log.Println(err)
		return err
	}
	os.Remove(filePath)
	return nil
}

func (g *File) SaveDataToGlobalStorage(storageRoot string, data []byte, key string, overwrite bool) error {
	var dirPath = storageRoot
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return err
	}
	var flags = 0
	filePath := fmt.Sprintf("%s/%s", dirPath, key)
	if overwrite {
		os.Remove(filePath)
		flags = os.O_WRONLY | os.O_CREATE
	} else {
		flags = os.O_APPEND | os.O_WRONLY | os.O_CREATE
	}
	dest, err := os.OpenFile(filePath, flags, 0600)
	if err != nil {
		return err
	}
	defer func(dest *os.File) {
		err := dest.Close()
		if err != nil {
			log.Println(err)
		}
	}(dest)
	if _, err = dest.Write(data); err != nil {
		return err
	}
	return nil
}

func NewFileTool(storageRoot string) *File {
	ft := &File{}
	var dirPath = fmt.Sprintf("%s/files", storageRoot)
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		log.Println(err)
	}
	return ft
}
