package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

var (
	dbClient   *mongo.Client
	dbClientMx sync.Mutex
)

func mongoGetClient() (*mongo.Client, error) {
	dbClientMx.Lock()
	defer dbClientMx.Unlock()
	if dbClient == nil {
		var err error
		opts := options.Client().ApplyURI(fmt.Sprintf("mongodb://localhost:%d/admin?connect=direct", cfg.MDBPort))
		dbClient, err = mongo.NewClient(opts)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err = dbClient.Connect(ctx)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = dbClient.Ping(ctx, readpref.Primary())
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return dbClient, nil
}

// mongoReplSetGetStatus gets the rs status for this mongo instance
// If it works with no errors, instances is in the rs
// If an error, instance is not in the rs
func mongoReplSetGetStatus() (map[string]interface{}, error) {
	c, err := mongoGetClient()
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{}
	err = c.Database("admin").RunCommand(context.Background(), map[string]interface{}{"replSetGetStatus": 1}).Decode(&result)
	return result, errors.WithStack(err)
}
