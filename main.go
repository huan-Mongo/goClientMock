package main

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	mt "go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"go.mongodb.org/mongo-driver/mongo/options"
	_ "go.mongodb.org/mongo-driver/mongo/options"
	"testmock.com/test/replitest"
)

var mockDeployment *replitest.MockDeployment

var started []*event.CommandStartedEvent
var succeeded []*event.CommandSucceededEvent
var failed []*event.CommandFailedEvent
var finishedEvents []*event.CommandFinishedEvent


func newTestClient() *mongo.Client {

	clientOpts := options.Client().SetWriteConcern(mt.MajorityWc).SetReadPreference(mt.PrimaryRp)

	// command monitor
	clientOpts.SetMonitor(&event.CommandMonitor{
		Started: func(_ context.Context, cse *event.CommandStartedEvent) {
			started = append(started, cse)
		},
		Succeeded: func(_ context.Context, cse *event.CommandSucceededEvent) {
			succeeded = append(succeeded, cse)
			finishedEvents = append(finishedEvents, &cse.CommandFinishedEvent)
		},
		Failed: func(_ context.Context, cfe *event.CommandFailedEvent) {
			failed = append(failed, cfe)
			finishedEvents = append(finishedEvents, &cfe.CommandFinishedEvent)
		},
	})
	connsCheckedOut := 0

	// only specify connection pool monitor if no deployment is given
	if clientOpts.Deployment == nil {
		previousPoolMonitor := clientOpts.PoolMonitor

		clientOpts.SetPoolMonitor(&event.PoolMonitor{
			Event: func(evt *event.PoolEvent) {
				if previousPoolMonitor != nil {
					previousPoolMonitor.Event(evt)
				}

				switch evt.Type {
				case event.GetSucceeded:
					connsCheckedOut++
				case event.ConnectionReturned:
					connsCheckedOut--
				}
			},
		})
	}

	clientOpts.PoolMonitor = nil
	mockDeployment = replitest.NewMockDeployment()
	clientOpts.Deployment = mockDeployment
	client, _:= mongo.NewClient(clientOpts)
	return client
}

func main()  {
	client := newTestClient()
	client.Connect(context.Background())

	mockDeployment.AddResponses(mt.CreateSuccessResponse())

	coll := client.Database("test").Collection("test")
	ret, err := coll.InsertOne(context.Background(), bson.D{})
	fmt.Println(ret)
	fmt.Println(err)

	mockDeployment.AddResponses(mt.CreateWriteErrorsResponse(mt.WriteError{
		Message: "Not transaction numbers",
		Code:    20,
		},
	))

	ret, err = coll.InsertOne(context.Background(), bson.D{})
	fmt.Println(ret)
	fmt.Println(err)

	ns := coll.Database().Name() + "." + coll.Name()
	aggregateRes := mt.CreateCursorResponse(1, ns, mt.FirstBatch, bson.D{
		{"_id", bson.D{{"first", "resume token"}}},
	})

	mockDeployment.AddResponses(aggregateRes)
	cs, err := coll.Watch(context.Background(), bson.D{})
	fmt.Println(err)
	if err == nil  {
		// if there was no error and an error is expected, capture the result from iterating the stream once
		fmt.Println(cs.Next(context.Background()))
		err = cs.Err()
		fmt.Println(err)
	}

	fmt.Printf("%+q\n", started)
	fmt.Printf("%+q\n", succeeded)
	fmt.Printf("%+q\n", failed)
	fmt.Printf("%+q\n", finishedEvents)
}