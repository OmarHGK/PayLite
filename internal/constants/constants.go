package constants

import "time"

// Database and Collection Names
const (
	DatabaseName            = "paylite"
	CollectionNameAccounts  = "accounts"
	CollectionNameTransfers = "transfers"
	CollectionNameLedger    = "ledger_entries"
)

// Transfer Status
const (
	TransferStatusCompleted = "completed"
)

// Context Timeouts
const (
	DefaultContextTimeout  = 5 * time.Second
	LongContextTimeout     = 10 * time.Second
	DatabaseContextTimeout = 15 * time.Second
)

// HTTP Server
const (
	ServerPort = ":8080"
)

// BSON Field Names
const (
	BSONFieldID             = "_id"
	BSONFieldBalanceCents   = "balance_cents"
	BSONFieldCreatedAt      = "created_at"
	BSONFieldUpdatedAt      = "updated_at"
	BSONFieldTransferID     = "transfer_id"
	BSONFieldAccountID      = "account_id"
	BSONFieldDirection      = "direction"
	BSONFieldAmountCents    = "amount_cents"
	BSONFieldCurrency       = "currency"
	BSONFieldIdempotencyKey = "idempotency_key"
)
