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
	"goClientMock/replitest"
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
	mockDeployment.AddResponses(mt.CreateSuccessResponse())

	coll := client.Database("test").Collection("test_col")
	coll.InsertMany(context.Background(), []interface{}{
		bson.D{
			{"_id", "1"},
		},
		bson.D{
			{"_id", "2"}},
		},
	)

	coll.InsertMany(context.Background(), []interface{}{
		bson.D{
			{"_id", "3"},
		},
		bson.D{
			{"_id", "4"}},
	},
	)

	cnt := 0
	for _, event := range started {
		if _, ok := event.Command.Lookup("insert").StringValueOK(); ok {
			if documents, ok := event.Command.Lookup("documents").ArrayOK(); ok {
				if doc, err := documents.Elements(); err == nil {
					//fmt.Println(doc)
					cnt += len(doc)
				}
			}
		}
	}
	fmt.Println(cnt)
}