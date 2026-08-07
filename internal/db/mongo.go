package db

import (
	"context"
<<<<<<< HEAD
<<<<<<< HEAD
	"errors"
	"log/slog"
=======
	"log"
	"time"
>>>>>>> main
=======
	"log"
	"time"
>>>>>>> df0ac3420447ee516647febebc9116964d3aa784

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/OmarHGK/paylite/internal/constants"
)

func Connect(uri string) *mongo.Client {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatalf("failed to connect to mongo: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), constants.LongContextTimeout)
	defer cancel()

	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("failed to ping mongo: %v", err)
	}

	log.Println("connected to mongodb")
	return client
}

func InitCollections(client *mongo.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), constants.DatabaseContextTimeout)
	defer cancel()

	paylite := client.Database(constants.DatabaseName)

	createLedgerCollection(ctx, paylite)
	createLedgerIndexes(ctx, paylite)
}

func createLedgerCollection(ctx context.Context, db *mongo.Database) {

	validator := bson.M{
		"$jsonSchema": bson.M{
			"bsonType": "object",
			"required": []string{
				"transfer_id",
				"account_id",
				"direction",
				"amount_cents",
				"currency",
				"balance_after_cents",
				"created_at",
			},
			"properties": bson.M{
				"transfer_id": bson.M{
					"bsonType":    "objectId",
					"description": "must be an ObjectId and is required",
				},
				"account_id": bson.M{
					"bsonType":    "objectId",
					"description": "must be an ObjectId and is required",
				},
				"direction": bson.M{

					"bsonType":    "string",
					"enum":        []string{"debit", "credit"},
					"description": "must be 'debit' or 'credit' and is required",
				},
				"amount_cents": bson.M{

					"bsonType":    "long",
					"description": "must be a long integer and is required",
				},
				"currency": bson.M{
					"bsonType":    "string",
					"description": "must be a string and is required",
				},
				"balance_after_cents": bson.M{
					"bsonType":    "long",
					"description": "must be a long integer and is required",
				},
				"created_at": bson.M{
					"bsonType":    "date",
					"description": "must be a date and is required",
				},
			},
		},
	}

	collectionOpts := options.CreateCollection().
		SetValidator(validator).
		SetValidationLevel("strict").
		SetValidationAction("error")

	err := db.CreateCollection(ctx, constants.CollectionNameLedger, collectionOpts)
	if err != nil {

		if !isNamespaceExistsError(err) {
			slog.Error("failed to create ledger_entries collection", "error", err)
			panic(err)
		}
		slog.Info("ledger_entries collection already exists, skipping creation")
		return
	}

	slog.Info("ledger_entries collection created with schema validator")
}

func createLedgerIndexes(ctx context.Context, db *mongo.Database) {
	ledger := db.Collection(constants.CollectionNameLedger)

	indexes := []mongo.IndexModel{
		{

			Keys: bson.D{
				{Key: constants.BSONFieldTransferID, Value: 1},
				{Key: constants.BSONFieldAccountID, Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetName("transfer_account_unique"),
		},
		{

			Keys: bson.D{
				{Key: constants.BSONFieldAccountID, Value: 1},
				{Key: constants.BSONFieldCreatedAt, Value: 1},
			},
			Options: options.Index().
				SetName("account_statement"),
		},
	}

	_, err := ledger.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		slog.Error("failed to create ledger_entries indexes", "error", err)
		panic(err)
	}

	slog.Info("ledger_entries indexes created")
}

func isNamespaceExistsError(err error) bool {

	var cmdErr mongo.CommandError
	if !errors.As(err, &cmdErr) {
		return false
	}

	return cmdErr.Code == 48
}
