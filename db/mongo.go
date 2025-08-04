package db

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	mongoClient *mongo.Client
)

// WebhookData represents a single webhook request
type WebhookData struct {
	ID          string                 `json:"id"`
	Timestamp   int64                  `json:"timestamp"`
	Method      string                 `json:"method"`
	Headers     map[string]string      `json:"headers"`
	Body        interface{}            `json:"body"`
	QueryParams map[string]string      `json:"queryParams"`
	UserAgent   string                 `json:"userAgent"`
	IP          string                 `json:"ip"`
	RawBody     string                 `json:"rawBody"`
}

func ConnectMongo(mongoURI string) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	mongoClient = client
	return client, nil
}

func InsertWebhookData(dbName, collectionName string, data *WebhookData) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	collection := mongoClient.Database(dbName).Collection(collectionName)
	_, err := collection.InsertOne(ctx, data)
	if err != nil {
		return fmt.Errorf("failed insert webhook data: %w", err)
	}
	return nil
}
