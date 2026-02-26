package packet

type ConsumeTokenInput struct {
	Orig         string `json:"orig" validate:"required"`
	TokenOwnerId string `json:"tokenOwnerId" validate:"required"`
	TokenId      string `json:"tokenId" validate:"required"`
	Amount       int64  `json:"amount" validate:"required"`
}