package security

type ISecurity interface {
	LoadKeys()
	GenerateSecureKeyPair(tag string)
	FetchKeyPair(tag string) [][]byte
	Encrypt(tag string, plainText string) string
	Decrypt(tag string, cipherText string) string
	AuthWithSignature(userId string, packet []byte, signatureBase64 string) (bool, string, bool)
	HasAccessToPoint(userId string, pointId string) bool
}
