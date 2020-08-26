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
	v1 "k8s.io/api/core/v1"
)

var (
	mongoLocalClient   *mongo.Client
	mongoLocalClientMx sync.Mutex
)

func mongoDirectConnect(addr string) (*mongo.Client, error) {
	opts := options.Client().ApplyURI(fmt.Sprintf("mongodb://%s:%d/admin?connect=direct", addr, cfg.MDBPort))
	c, err := mongo.NewClient(opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = c.Connect(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = c.Ping(ctx, readpref.Primary())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return c, nil
}

func mongoGetLocalClient() (*mongo.Client, error) {
	mongoLocalClientMx.Lock()
	defer mongoLocalClientMx.Unlock()
	if mongoLocalClient == nil {
		var err error
		mongoLocalClient, err = mongoDirectConnect("localhost")
		if err != nil {
			return nil, err
		}
	}
	return mongoLocalClient, nil
}

// mongoReplSetGetStatus gets the rs status for a mongo instance
// If it works with no errors, instances is in the rs
// If an error, instance is not in the rs
// Passing nil gets the status of the localhost instance.
func mongoReplSetGetStatus(c *mongo.Client) (map[string]interface{}, error) {
	var err error
	if c == nil {
		c, err = mongoGetLocalClient()
		if err != nil {
			return nil, err
		}
	}
	result := map[string]interface{}{}
	err = c.Database("admin").RunCommand(context.Background(), map[string]interface{}{"replSetGetStatus": 1}).Decode(&result)
	return result, errors.WithStack(err)
}

func mongoIsInReplSet(pod *v1.Pod) (bool, error) {
	c, err := mongoDirectConnect(podFQDN(pod))
	if err != nil {
		return false, err
	}
	defer func() { _ = c.Disconnect(context.Background()) }()

	result := map[string]interface{}{}
	err = c.Database("admin").RunCommand(context.Background(), map[string]interface{}{"replSetGetStatus": 1}).Decode(&result)
	return err == nil, errors.WithStack(err)
}

// mongoInitReplSet initializes the replica set from localhost.
func mongoInitReplSet(pods []v1.Pod) error {
	c, err := mongoGetLocalClient()
	if err != nil {
		return err
	}
	var members []map[string]interface{}
	for _, pod := range pods {
		ordinal, err := podOrd(&pod)
		if err != nil {
			return err
		}
		members = append(members, map[string]interface{}{
			"_id":  ordinal,
			"host": podFQDN(&pod) + ":57017",
		})
	}
	cmd := map[string]interface{}{
		"replSetInitiate": map[string]interface{}{
			"_id":     "rs0",
			"members": members,
		}}
	sr := c.Database("admin").RunCommand(context.Background(), cmd)
	return errors.WithStack(sr.Err())
}
