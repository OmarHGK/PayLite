package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Transfer struct {
	ID             bson.ObjectID `json:"id" bson:"_id,omitempty"`
	FromAccountID  bson.ObjectID `json:"from_account_id" bson:"from_account"`
	ToAccountID    bson.ObjectID `json:"to_account_id" bson:"to_account"`
	AmountCents    int64         `json:"amount_cents" bson:"amount_cents"`
	Status         string        `json:"status" bson:"status"`
	IdempotencyKey string        `json:"idempotency_key" bson:"idempotency_key"`
	CreatedAt      time.Time     `json:"created_at" bson:"created_at"`
}
