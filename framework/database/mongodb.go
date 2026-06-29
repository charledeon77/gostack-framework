/*
GoMon — GoStack MongoDB Integration Subsystem.

Purpose:
This file implements the GoMon MongoDB initialization helper for the GoStack framework.
It exposes a *mongo.Client bound to the global gostack.Mongo facade after connecting
and pinging the MongoDB server to verify the connection is alive.

Philosophy:
GoMon does not wrap the MongoDB client behind a custom interface. Instead, it surfaces
the official mongo.Client directly, giving developers full access to the native MongoDB
Go driver API (collections, aggregations, transactions, change streams, etc.) without
any limitations imposed by an intermediate wrapper layer.

Architecture:
GoMon sits in the database package alongside the SQL adapter. It is only imported and
compiled into the application binary when the developer explicitly selects MongoDB during
project scaffolding. This upholds GoStack's Modular Self-Containment principle.
*/
package database

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// InitGoMon initializes and verifies a MongoDB connection using the provided URI.
// It returns the connected *mongo.Client ready for use as the global gostack.Mongo facade.
//
// Parameters:
//   - uri: The MongoDB connection string (e.g. "mongodb://127.0.0.1:27017").
//
// Returns:
//   - A live *mongo.Client, or an error if connection or ping fails.
func InitGoMon(uri string) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("[GoMon] Failed to connect to MongoDB: %w", err)
	}

	// Ping the server to verify the connection is alive.
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("[GoMon] MongoDB ping failed: %w", err)
	}

	return client, nil
}
