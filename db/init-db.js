db = db.getSiblingDB("paylite");

db.accounts.drop();
db.transfers.drop();

db.createCollection("accounts", {
  validator: {
    $jsonSchema: {
      bsonType: "object",
      required: ["owner_name", "balance_cents", "currency", "created_at", "updated_at"],
      properties: {
        owner_name: {
          bsonType: "string",
          description: "must be a string and is required"
        },
        balance_cents: {
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
  validationLevel: "strict",
  validationAction: "error"
});

db.createCollection("transfers", {
  validator: {
    $jsonSchema: {
      bsonType: "object",
      required: ["from_account", "to_account", "amount_cents", "status", "idempotency_key", "created_at"],
      properties: {
        from_account: {
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

db.transfers.createIndex(
  { idempotency_key: 1 },
  { unique: true, name: "uniq_idempotency_key" }
);

print("paylite schema initialized: accounts, transfers, and indexes created.");
