# \# PayLite

# 

# A backend payment and transfer service built in Go, demonstrating double-entry bookkeeping, atomic transactions, and Stripe payment integration.

# 

# Built as a capstone project during a backend engineering internship at MNT-Halan.

# 

# \---

# 

# \## Tech Stack

# 

# | Layer | Technology |

# |---|---|

# | Language | Go 1.26 |

# | Database | MongoDB 7 (replica set required for transactions) |

# | Payments | Stripe API (sandbox) |

# | Containerization | Docker \& Docker Compose |

# | Logging | `log/slog` (JSON structured output) |

# 

# \---

# 

# \## Features

# 

# \- \*\*Accounts\*\* — Create accounts with an initial balance and retrieve them by ID

# \- \*\*Transfers\*\* — Peer-to-peer money transfers with idempotency keys and double-entry bookkeeping; atomic via MongoDB transactions

# \- \*\*Ledger\*\* — Every transfer produces two ledger entries (debit + credit) with running balance snapshots

# \- \*\*Payments\*\* — Stripe sandbox payment intent creation and server-side confirmation that credits the account on success

# \- \*\*Observability\*\* — JSON structured logging, `X-Request-ID` tracing middleware, `/health` endpoint

# \- \*\*Operationalization\*\* — Docker Compose setup, graceful shutdown, environment-based configuration, MongoDB `$jsonSchema` validators

# 

# \---

# 

# \## Getting Started

# 

# \### Prerequisites

# 

# \- Go 1.26+

# \- Docker \& Docker Compose

# \- A Stripe sandbox secret key (from \[dashboard.stripe.com/test/apikeys](https://dashboard.stripe.com/test/apikeys))

# 

# \### Run with Docker Compose

# 

# ```bash

# git clone https://github.com/OmarHGK/paylite.git

# cd paylite

# 

# \# Create your env file (STRIPE\_SECRET\_KEY is required)

# cp .env.example .env

# \# then edit .env and fill in your Stripe key

# 

# docker-compose up

# ```

# 

# The API will be available at `http://localhost:8080`.

# 

# \### Environment Variables

# 

# | Variable | Required | Default | Description |

# |---|---|---|---|

# | `STRIPE\_SECRET\_KEY` | \*\*Yes\*\* | — | Stripe sandbox secret key |

# | `MONGO\_URI` | No | `mongodb://localhost:27017` | MongoDB connection string |

# | `PORT` | No | `8080` | HTTP server port |

# 

# \### Verify

# 

# ```bash

# curl http://localhost:8080/health

# \# {"status":"ok"}

# ```

# 

# \---

# 

# \## API Reference

# 

# \### Health

# 

# ```

# GET /health

# → 200 {"status": "ok"}

# ```

# 

# \---

# 

# \### Accounts

# 

# \*\*Create Account\*\*

# 

# ```

# POST /accounts

# ```

# 

# Request:

# ```json

# {

# &#x20; "owner\_name": "Alice",

# &#x20; "currency": "USD",

# &#x20; "balance\_cents": 10000

# }

# ```

# 

# Response `201`:

# ```json

# {

# &#x20; "id": "6883a1...",

# &#x20; "owner\_name": "Alice",

# &#x20; "balance\_cents": 10000,

# &#x20; "currency": "USD",

# &#x20; "created\_at": "2026-07-29T10:00:00Z",

# &#x20; "updated\_at": "2026-07-29T10:00:00Z"

# }

# ```

# 

# \---

# 

# \*\*Get Account\*\*

# 

# ```

# GET /accounts/{id}

# ```

# 

# Response `200`:

# ```json

# {

# &#x20; "id": "6883a1...",

# &#x20; "owner\_name": "Alice",

# &#x20; "balance\_cents": 10000,

# &#x20; "currency": "USD",

# &#x20; "created\_at": "2026-07-29T10:00:00Z",

# &#x20; "updated\_at": "2026-07-29T10:00:00Z"

# }

# ```

# 

# \---

# 

# \### Transfers

# 

# \*\*Create Transfer\*\*

# 

# Transfers funds between two accounts atomically. If `idempotency\_key` was already used, returns the original transfer without processing again.

# 

# ```

# POST /transfers

# ```

# 

# Request:

# ```json

# {

# &#x20; "from\_account": "6883a1...",

# &#x20; "to\_account": "6883b2...",

# &#x20; "amount\_cents": 2500,

# &#x20; "idempotency\_key": "pay-order-42"

# }

# ```

# 

# Response `201`:

# ```json

# {

# &#x20; "id": "6883c3...",

# &#x20; "from\_account\_id": "6883a1...",

# &#x20; "to\_account\_id": "6883b2...",

# &#x20; "amount\_cents": 2500,

# &#x20; "status": "completed",

# &#x20; "idempotency\_key": "pay-order-42",

# &#x20; "created\_at": "2026-07-29T10:01:00Z"

# }

# ```

# 

# Error responses:

# \- `400` — missing or invalid fields

# \- `422` — insufficient funds in source account

# \- `200` — idempotent replay (returns original transfer)

# 

# \---

# 

# \### Payments (Stripe)

# 

# \*\*Create Payment\*\*

# 

# Creates a Stripe PaymentIntent in sandbox mode and stores a payment record linked to the account.

# 

# ```

# POST /payments

# ```

# 

# Request:

# ```json

# {

# &#x20; "account\_id": "6883a1...",

# &#x20; "amount\_cents": 5000,

# &#x20; "currency": "usd"

# }

# ```

# 

# Response `201`:

# ```json

# {

# &#x20; "payment\_id": "6883d4...",

# &#x20; "client\_secret": "pi\_...secret\_...",

# &#x20; "amount\_cents": 5000,

# &#x20; "currency": "usd",

# &#x20; "status": "requires\_payment\_method"

# }

# ```

# 

# \---

# 

# \*\*Confirm Payment\*\*

# 

# Verifies the PaymentIntent status with Stripe. On success, atomically credits the account balance and marks the payment as `succeeded`.

# 

# ```

# POST /payments/confirm

# ```

# 

# Request:

# ```json

# {

# &#x20; "payment\_id": "6883d4...",

# &#x20; "stripe\_intent\_id": "pi\_..."

# }

# ```

# 

# Response `200`:

# ```json

# {

# &#x20; "success": true,

# &#x20; "payment\_id": "6883d4...",

# &#x20; "status": "succeeded",

# &#x20; "balance\_cents": 15000,

# &#x20; "message": "Payment of 5000 cents successfully applied"

# }

# ```

# 

# Error responses:

# \- `400` — invalid IDs or intent ID mismatch

# \- `402` — Stripe reports payment not succeeded (includes failure reason)

# \- `404` — payment record not found

# 

# \---

# 

# \## Project Structure

# 

# ```

# paylite/

# ├── main.go                       # Entry point: routing, server setup

# ├── go.mod

# ├── docker-compose.yml

# ├── .env.example

# ├── db/

# │   └── init-db.js               # MongoDB schema validation setup

# └── internal/

# &#x20;   ├── config/

# &#x20;   │   └── config.go            # Environment-based config loading

# &#x20;   ├── constants/

# &#x20;   │   └── constants.go         # DB names, timeouts, BSON field names

# &#x20;   ├── db/

# &#x20;   │   └── mongo.go             # MongoDB client initialization

# &#x20;   ├── handlers/

# &#x20;   │   ├── accounts.go          # CreateAccount, GetAccount

# &#x20;   │   ├── transfers.go         # CreateTransfer (atomic, idempotent)

# &#x20;   │   ├── payments.go          # CreatePayment, ConfirmPayment (Stripe)

# &#x20;   │   ├── ledger.go            # GetAccountStatement, GetTransferEntries

# &#x20;   │   └── ledger\_test.go       # Integration tests for ledger correctness

# &#x20;   ├── middleware/

# &#x20;   │   └── request\_id.go        # X-Request-ID extraction

# &#x20;   └── models/

# &#x20;       ├── account.go

# &#x20;       ├── transfer.go

# &#x20;       ├── ledger\_entry.go      # Direction (debit/credit), BalanceAfterCents

# &#x20;       └── payment.go           # StripePaymentID, FailureReason

# ```

# 

# \---

# 

# \## Design Decisions

# 

# \### Double-Entry Bookkeeping

# 

# Every transfer writes two ledger entries inside the same MongoDB transaction:

# 

# \- \*\*Debit\*\* on the source account (`amount\_cents` is negative)

# \- \*\*Credit\*\* on the destination account (`amount\_cents` is positive)

# 

# Each entry captures the `balance\_after\_cents` at the time of the transaction, giving a point-in-time snapshot for auditing. The sum of all ledger entries for an account equals its current balance.

# 

# \### MongoDB Transactions

# 

# Transfers are implemented as multi-document transactions spanning three writes: debit the source account, credit the destination account, insert two ledger entries. If any step fails the entire transaction rolls back — no partial state is ever committed.

# 

# A replica set is required for MongoDB transactions, which is why `docker-compose.yml` starts Mongo with `--replSet rs0`.

# 

# \### Idempotency

# 

# The transfer handler checks for an existing record with the given `idempotency\_key` before opening a transaction. If found, it returns the stored result immediately. A unique index on `idempotency\_key` catches any race conditions at the database level.

# 

# \### Stripe Confirmation Flow

# 

# Payment creation and confirmation are kept as two separate endpoints to mirror how Stripe's client-side SDK works in a real frontend integration:

# 

# 1\. Backend creates a PaymentIntent and returns the `client\_secret` to the frontend

# 2\. Frontend uses the `client\_secret` to complete payment (not implemented here)

# 3\. Backend confirms by fetching the PaymentIntent status directly from Stripe, then atomically credits the account

# 

# `AllowRedirects: "never"` is set on the PaymentIntent so only card-based methods are accepted in sandbox.

# 

# \---

# 

# \## Running Tests

# 

# ```bash

# go test ./internal/handlers/...

# ```

# 

# The test suite covers ledger consistency: it verifies that after a transfer the debit and credit entries sum to zero, and that each entry's `balance\_after\_cents` matches the account's final balance.

# 

# \---

# 

# \## Branch History

# 

# Each branch corresponds to one week of development:

# 

# | Branch | What was built |

# |---|---|

# | `week2/01-schema-design` | MongoDB collections with `$jsonSchema` validators and indexes |

# | `week2/02-accounts-transfers-api` | REST API for accounts and transfers |

# | `week2/04-docker-compose-ops` | Docker Compose, structured logging, request ID middleware, graceful shutdown |

# | `week3/01-ledger-implementation` | Double-entry ledger with atomic writes and integration tests |

# | `week4/01-Stripe-Integration` | Stripe sandbox payment creation and server-side confirmation |

