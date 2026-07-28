package config

import (
	"fmt"
	"os"
)

type Config struct {
	StripeSecretKey string
	MongoURI        string
	Port            string
}

func LoadConfig() (*Config, error) {
	stripeSecretKey := os.Getenv("STRIPE_SECRET_KEY")
	if stripeSecretKey == "" {
		return nil, fmt.Errorf("STRIPE_SECRET_KEY environment variable not set")
	}

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return &Config{
		StripeSecretKey: stripeSecretKey,
		MongoURI:        mongoURI,
		Port:            port,
	}, nil
}
