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

