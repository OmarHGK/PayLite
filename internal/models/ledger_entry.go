package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Direction string

const (
	DirectionDebit  Direction = "debit"
	DirectionCredit Direction = "credit"
)

type LedgerEntry struct {
	ID bson.ObjectID `json:"id" bson:"_id,omitempty"`

	TransferID bson.ObjectID `json:"transfer_id" bson:"transfer_id"`

	AccountID bson.ObjectID `json:"account_id" bson:"account_id"`

	Direction Direction `json:"direction" bson:"direction"`

	AmountCents int64 `json:"amount_cents" bson:"amount_cents"`

	Currency string `json:"currency" bson:"currency"`

	BalanceAfterCents int64 `json:"balance_after_cents" bson:"balance_after_cents"`

	CreatedAt time.Time `json:"created_at" bson:"created_at"`
}
