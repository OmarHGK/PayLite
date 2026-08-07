<<<<<<< HEAD
# PayLite

A backend payment and transfer service built in Go, demonstrating double-entry bookkeeping, atomic transactions, and Stripe payment integration.

Built as a capstone project during a backend engineering internship at MNT-Halan.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.26 |
| Database | MongoDB 7 (replica set required for transactions) |
| Payments | Stripe API (sandbox) |
| Containerization | Docker & Docker Compose |
| Logging | `log/slog` (JSON structured output) |

---

## Features

- **Accounts** — Create accounts with an initial balance and retrieve them by ID
- **Transfers** — Peer-to-peer money transfers with idempotency keys and double-entry bookkeeping; atomic via MongoDB transactions
- **Ledger** — Every transfer produces two ledger entries (debit + credit) with running balance snapshots
- **Payments** — Stripe sandbox payment intent creation and server-side confirmation that credits the account on success
- **Observability** — JSON structured logging, `X-Request-ID` tracing middleware, `/health` endpoint
- **Operationalization** — Docker Compose setup, graceful shutdown, environment-based configuration, MongoDB `$jsonSchema` validators

---

## Getting Started

### Prerequisites

- Go 1.26+
- Docker & Docker Compose
- A Stripe sandbox secret key (from [dashboard.stripe.com/test/apikeys](https://dashboard.stripe.com/test/apikeys))

### Run with Docker Compose

```bash
git clone https://github.com/OmarHGK/paylite.git
cd paylite

# Create your env file (STRIPE_SECRET_KEY is required)
cp .env.example .env
# then edit .env and fill in your Stripe key

docker-compose up
```

The API will be available at `http://localhost:8080`.

### Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `STRIPE_SECRET_KEY` | **Yes** | — | Stripe sandbox secret key |
| `MONGO_URI` | No | `mongodb://localhost:27017` | MongoDB connection string |
| `PORT` | No | `8080` | HTTP server port |

### Verify

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

---

## API Reference

### Health

```
GET /health
→ 200 {"status": "ok"}
```

---

### Accounts

**Create Account**

```
POST /accounts
```

Request:
```json
{
  "owner_name": "Alice",
  "currency": "USD",
  "balance_cents": 10000
}
```

Response `201`:
```json
{
  "id": "6883a1...",
  "owner_name": "Alice",
  "balance_cents": 10000,
  "currency": "USD",
  "created_at": "2026-07-29T10:00:00Z",
  "updated_at": "2026-07-29T10:00:00Z"
}
```

---

**Get Account**

```
GET /accounts/{id}
```

Response `200`:
```json
{
  "id": "6883a1...",
  "owner_name": "Alice",
  "balance_cents": 10000,
  "currency": "USD",
  "created_at": "2026-07-29T10:00:00Z",
  "updated_at": "2026-07-29T10:00:00Z"
}
```

---

### Transfers

**Create Transfer**

Transfers funds between two accounts atomically. If `idempotency_key` was already used, returns the original transfer without processing again.

```
POST /transfers
```

Request:
```json
{
  "from_account": "6883a1...",
  "to_account": "6883b2...",
  "amount_cents": 2500,
  "idempotency_key": "pay-order-42"
}
```

Response `201`:
```json
{
  "id": "6883c3...",
  "from_account_id": "6883a1...",
  "to_account_id": "6883b2...",
  "amount_cents": 2500,
  "status": "completed",
  "idempotency_key": "pay-order-42",
  "created_at": "2026-07-29T10:01:00Z"
}
```

Error responses:
- `400` — missing or invalid fields
- `422` — insufficient funds in source account
- `200` — idempotent replay (returns original transfer)

---

### Payments (Stripe)

**Create Payment**

Creates a Stripe PaymentIntent in sandbox mode and stores a payment record linked to the account.

```
POST /payments
```

Request:
```json
{
  "account_id": "6883a1...",
  "amount_cents": 5000,
  "currency": "usd"
}
```

Response `201`:
```json
{
  "payment_id": "6883d4...",
  "client_secret": "pi_...secret_...",
  "amount_cents": 5000,
  "currency": "usd",
  "status": "requires_payment_method"
}
```

---

**Confirm Payment**

Verifies the PaymentIntent status with Stripe. On success, atomically credits the account balance and marks the payment as `succeeded`.

```
POST /payments/confirm
```

Request:
```json
{
  "payment_id": "6883d4...",
  "stripe_intent_id": "pi_..."
}
```

Response `200`:
```json
{
  "success": true,
  "payment_id": "6883d4...",
  "status": "succeeded",
  "balance_cents": 15000,
  "message": "Payment of 5000 cents successfully applied"
}
```

Error responses:
- `400` — invalid IDs or intent ID mismatch
- `402` — Stripe reports payment not succeeded (includes failure reason)
- `404` — payment record not found

---

## Project Structure

```
paylite/
├── main.go                       # Entry point: routing, server setup
├── go.mod
├── docker-compose.yml
├── .env.example
├── db/
│   └── init-db.js               # MongoDB schema validation setup
└── internal/
    ├── config/
    │   └── config.go            # Environment-based config loading
    ├── constants/
    │   └── constants.go         # DB names, timeouts, BSON field names
    ├── db/
    │   └── mongo.go             # MongoDB client initialization
    ├── handlers/
    │   ├── accounts.go          # CreateAccount, GetAccount
    │   ├── transfers.go         # CreateTransfer (atomic, idempotent)
    │   ├── payments.go          # CreatePayment, ConfirmPayment (Stripe)
    │   ├── ledger.go            # GetAccountStatement, GetTransferEntries
    │   └── ledger_test.go       # Integration tests for ledger correctness
    ├── middleware/
    │   └── request_id.go        # X-Request-ID extraction
    └── models/
        ├── account.go
        ├── transfer.go
        ├── ledger_entry.go      # Direction (debit/credit), BalanceAfterCents
        └── payment.go           # StripePaymentID, FailureReason
```

---

## Design Decisions

### Double-Entry Bookkeeping

Every transfer writes two ledger entries inside the same MongoDB transaction:

- **Debit** on the source account (`amount_cents` is negative)
- **Credit** on the destination account (`amount_cents` is positive)

Each entry captures the `balance_after_cents` at the time of the transaction, giving a point-in-time snapshot for auditing. The sum of all ledger entries for an account equals its current balance.

### MongoDB Transactions

Transfers are implemented as multi-document transactions spanning three writes: debit the source account, credit the destination account, insert two ledger entries. If any step fails the entire transaction rolls back — no partial state is ever committed.

A replica set is required for MongoDB transactions, which is why `docker-compose.yml` starts Mongo with `--replSet rs0`.

### Idempotency

The transfer handler checks for an existing record with the given `idempotency_key` before opening a transaction. If found, it returns the stored result immediately. A unique index on `idempotency_key` catches any race conditions at the database level.

### Stripe Confirmation Flow

Payment creation and confirmation are kept as two separate endpoints to mirror how Stripe's client-side SDK works in a real frontend integration:

1. Backend creates a PaymentIntent and returns the `client_secret` to the frontend
2. Frontend uses the `client_secret` to complete payment (not implemented here)
3. Backend confirms by fetching the PaymentIntent status directly from Stripe, then atomically credits the account

`AllowRedirects: "never"` is set on the PaymentIntent so only card-based methods are accepted in sandbox.

---

## Running Tests

```bash
go test ./internal/handlers/...
```

The test suite covers ledger consistency: it verifies that after a transfer the debit and credit entries sum to zero, and that each entry's `balance_after_cents` matches the account's final balance.

---

## Branch History

Each branch corresponds to one week of development:

| Branch | What was built |
|---|---|
| `week2/01-schema-design` | MongoDB collections with `$jsonSchema` validators and indexes |
| `week2/02-accounts-transfers-api` | REST API for accounts and transfers |
| `week2/04-docker-compose-ops` | Docker Compose, structured logging, request ID middleware, graceful shutdown |
| `week3/01-ledger-implementation` | Double-entry ledger with atomic writes and integration tests |
| `week4/01-Stripe-Integration` | Stripe sandbox payment creation and server-side confirmation |
=======
\# PayLite



A minimal payments/transfers backend built as a capstone project, moving from

standalone Go programs to an operable service backed by MongoDB.



Money is always stored as `int64` cents — never as a floating-point type —

to avoid rounding drift over many transactions.



\## Status



\- \[x] Week 2 / Task 01 — `accounts` and `transfers` collection schemas,

&#x20;     `$jsonSchema` validators, and unique indexes

\- \[ ] Week 2 / Task 02 — REST API (`POST /accounts`, `GET /accounts/{id}`,

&#x20;     `POST /transfers`) using the official Go MongoDB driver, transfers run

&#x20;     inside multi-document transactions

\- \[ ] Week 2 / Task 03 — reproduce and fix the week-1 `Transfer` race

&#x20;     condition using atomic `$inc` + transaction retry on write conflict

\- \[ ] Week 2 / Task 04 — `docker compose` brings up API + MongoDB

&#x20;     (single-node replica set) + indexes from a clean machine; structured

&#x20;     logs, request IDs, graceful shutdown



\## Requirements



\- Docker Desktop (WSL2 backend)

\- Go (added in Task 02)



\## Database setup



MongoDB must run as a single-node replica set (required for multi-document

transactions):



```bash

docker run -d --name paylite-mongo -p 27017:27017 mongo:7 --replSet rs0

docker exec -it paylite-mongo mongosh --eval "rs.initiate()"

```



Then initialize the schema:



```bash

docker cp db/init-db.js paylite-mongo:/init-db.js

docker exec -it paylite-mongo mongosh paylite /init-db.js

```



\## Schema design



See \[`db/init-db.js`](./db/init-db.js) for the full `$jsonSchema` validators

and index definitions.



\*\*`accounts`\*\* — one document per account. `balance\_cents` is a required

64-bit integer (`long`); `owner\_name`, `currency`, `created\_at`, and

`updated\_at` are also required.



\*\*`transfers`\*\* — one document per transfer attempt. `amount\_cents` is a

required 64-bit integer. `status` is a required string enum of `pending`,

`completed`, or `failed`. `idempotency\_key` is a required string with a

\*\*unique index\*\*, so a retried request can't double-process the same

transfer.



Both collections use `validationLevel: "strict"` and

`validationAction: "error"`, so any document that doesn't match the schema

is rejected outright rather than merely logged.

>>>>>>> main
