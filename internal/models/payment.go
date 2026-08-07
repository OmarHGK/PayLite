package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Payment struct {
	ID              bson.ObjectID `json:"id" bson:"_id,omitempty"`
	AccountID       bson.ObjectID `json:"account_id" bson:"account_id"`
	StripePaymentID string        `json:"stripe_payment_id" bson:"stripe_payment_id"`
	AmountCents     int64         `json:"amount_cents" bson:"amount_cents"`
	Currency        string        `json:"currency" bson:"currency"`
	Status          string        `json:"status" bson:"status"`
	FailureReason   string        `json:"failure_reason,omitempty" bson:"failure_reason,omitempty"`
	CreatedAt       time.Time     `json:"created_at" bson:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at" bson:"updated_at"`
}
