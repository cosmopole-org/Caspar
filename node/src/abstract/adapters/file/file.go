package file

import (
	"archive/tar"
	"mime/multipart"
)

type IFile interface {
	CheckFileFromStorage(storageRoot string, pointId string, key string) bool
	SaveFileToStorage(storageRoot string, fh *multipart.FileHeader, pointId string, key string) error
	SaveDataToStorage(storageRoot string, data []byte, pointId string, key string, flag... bool) error
	SaveTarFileItemToStorage(storageRoot string, fh *tar.Reader, pointId string, key string) error
	ReadFileFromStorage(storageRoot string, pointId string, key string) ([]byte, error)
	CheckFileFromGlobalStorage(storageRoot string, key string) bool
	ReadFileFromGlobalStorage(storageRoot string, key string) (string, error)
	SaveFileToGlobalStorage(storageRoot string, fh *multipart.FileHeader, key string, overwrite bool) error
	SaveDataToGlobalStorage(storageRoot string, data []byte, key string, overwrite bool) error
	DeleteFileFromGlobalStorage(storageRoot string, key string, overwrite bool) error
	ReadFileByPath(path string) ([]byte, error)
}
