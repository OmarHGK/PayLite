// Switch into (and implicitly create) the "paylite" database.
// Mongo doesn't require databases to be explicitly created ahead of time —
// they come into existence the first time something is written to them.
db = db.getSiblingDB("paylite");

// Drop both collections first so this script can be re-run safely during
// development without erroring on "collection already exists."
db.accounts.drop();
db.transfers.drop();

// Create the "accounts" collection with a validator attached.
// A validator is a rulebook MongoDB enforces on every write to this
// collection — without it, Mongo would accept documents of any shape.
db.createCollection("accounts", {
  validator: {
    $jsonSchema: {
      bsonType: "object",
      // These five fields must ALL be present, or the write is rejected.
      required: ["owner_name", "balance_cents", "currency", "created_at", "updated_at"],
      properties: {
        owner_name: {
          bsonType: "string",
          description: "must be a string and is required"
        },
        balance_cents: {
          // "long" = 64-bit integer. This is the enforcement mechanism for
          // "money is never stored as a float" — a double would be rejected.
          bsonType: "long",
          description: "must be a 64-bit integer (cents) and is required"
        },
        currency: {
          bsonType: "string",
          description: "must be a string and is required"
        },
        created_at: {
          bsonType: "date",
          description: "must be a date and is required"
        },
        updated_at: {
          bsonType: "date",
          description: "must be a date and is required"
        }
      }
    }
  },
  // "strict" = validate every insert/update, no exceptions.
  validationLevel: "strict",
  // "error" = reject bad documents outright, rather than just warning.
  validationAction: "error"
});

// Create the "transfers" collection, same pattern, with its own fields.
db.createCollection("transfers", {
  validator: {
    $jsonSchema: {
      bsonType: "object",
      required: ["from_account", "to_account", "amount_cents", "status", "idempotency_key", "created_at"],
      properties: {
        from_account: {
          // objectId = must specifically be a Mongo ObjectId, since this
          // references a real document in the accounts collection.
          bsonType: "objectId",
          description: "must be an ObjectId referencing the source account and is required"
        },
        to_account: {
          bsonType: "objectId",
          description: "must be an ObjectId referencing the destination account and is required"
        },
        amount_cents: {
          bsonType: "long",
          description: "must be a 64-bit integer (cents) and is required"
        },
        status: {
          bsonType: "string",
          // enum restricts this field to ONLY these three exact values —
          // anything else (a typo, wrong casing) gets rejected.
          enum: ["pending", "completed", "failed"],
          description: "must be one of pending/completed/failed and is required"
        },
        idempotency_key: {
          bsonType: "string",
          description: "must be a string and is required"
        },
        created_at: {
          bsonType: "date",
          description: "must be a date and is required"
        }
      }
    }
  },
  validationLevel: "strict",
  validationAction: "error"
});

// The one constraint with real teeth: no two transfer documents may ever
// share the same idempotency_key. This is what makes retried client
// requests safe — a duplicate insert fails instead of double-moving money.
db.transfers.createIndex(
  { idempotency_key: 1 },
  { unique: true, name: "uniq_idempotency_key" }
);

print("paylite schema initialized: accounts, transfers, and indexes created.");