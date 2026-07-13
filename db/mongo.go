package db

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options" // Run: go get go.mongodb.org/mongo-driver/mongo
)

type MongoService struct {
	client     *mongo.Client
	collection *mongo.Collection
}

func NewMongoService(uri, dbName, collName string) (*MongoService, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	collection := client.Database(dbName).Collection(collName)

	// Create a Text Index on the body field to unlock advanced search indexing capabilities
	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "body", Value: "text"}},
	}
	_, _ = collection.Indexes().CreateOne(ctx, indexModel)

	return &MongoService{client: client, collection: collection}, nil
}

func (m *MongoService) SavePayload(id string, body string, headers map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	doc := bson.M{
		"_id":        id,
		"body":       body,
		"headers":    headers,
		"created_at": time.Now(),
	}

	_, err := m.collection.InsertOne(ctx, doc)
	return err
}

// Payload is the full stored content of a received email.
type Payload struct {
	ID      string            `bson:"_id" json:"id"`
	Body    string            `bson:"body" json:"body"`
	Headers map[string]string `bson:"headers" json:"headers"`
}

// GetPayload fetches the full raw content of a previously saved email by id.
func (m *MongoService) GetPayload(id string) (*Payload, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var p Payload
	if err := m.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (m *MongoService) Close() error {
	return m.client.Disconnect(context.Background())
}
