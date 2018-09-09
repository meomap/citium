package main

import (
	"context"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/pkg/errors"

	"github.com/meomap/citium/config"
	"github.com/meomap/citium/scheduler"
)

func handler(conf *config.Configuration, conn dynamodbiface.DynamoDBAPI, client scheduler.Requester) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return errors.Wrap(scheduler.TriggerAPI(ctx, conf, conn, client), "scheduler.TriggerAPI")
	}
}

func main() {
	conf := config.Must(config.NewConfiguration())
	dbconn := dynamodb.New(session.Must(session.NewSession(nil)))
	client := scheduler.Must(scheduler.NewClient(conf))
	lambda.Start(handler(conf, dbconn, client))
}
