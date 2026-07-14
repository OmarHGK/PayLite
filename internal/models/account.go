package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Account struct {
	ID           bson.ObjectID `json:"id" bson:"_id,omitempty"`
	OwnerName    string        `json:"owner_name" bson:"owner_name"`
	BalanceCents int64         `json:"balance_cents" bson:"balance_cents"`
	CreatedAt    time.Time     `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at" bson:"updated_at"`
	Currency     string        `json:"currency" bson:"currency"`
}
