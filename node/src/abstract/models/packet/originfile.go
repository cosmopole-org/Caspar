package packet

import (
	"mime/multipart"
)

type OriginFile struct {
	FileInfo  string
	UserId    string
	SpaceId   string
	TopicId   string
	RequestId string
	Data      *multipart.FileHeader
}
