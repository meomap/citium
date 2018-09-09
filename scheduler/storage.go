package scheduler

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/pkg/errors"

	"github.com/meomap/citium/schema"
)

const unixFormat = "2006-01-02T15:04:05Z"

// FetchSchedRequests lookup for all the scheduled records from dynamodb matching the conditions:
// - EffectiveAfter >= time.Now().Unix()
// - Locking == false
func FetchSchedRequests(ctx context.Context, conn dynamodbiface.DynamoDBAPI, tableName string, current time.Time) ([]*schema.ScheduledRequest, error) {
	currentStr := current.Format(unixFormat)
	input := &dynamodb.ScanInput{
		TableName:        aws.String(tableName),
		FilterExpression: aws.String("EffectiveAfter <= :d and Locking = :l"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":d": {
				S: aws.String(currentStr),
			},
			":l": {
				BOOL: aws.Bool(false),
			},
		},
	}
	log.Printf("fetch the scheduled requests table_name=%s current=%s \n", tableName, currentStr)
	output, err := conn.Scan(input)
	if err != nil {
		return nil, errors.Wrapf(err, "conn.Scan table_name=%s input=%s", tableName, input.GoString())
	}
	log.Printf("found %d records\n", len(output.Items))
	records := []*schema.ScheduledRequest{}
	if err = dynamodbattribute.UnmarshalListOfMaps(output.Items, &records); err != nil {
		return nil, errors.Wrapf(err, "dynamodbattribute.UnmarshalListOfMaps table_name=%s output=%s", tableName, output.GoString())
	}
	return records, nil
}

// Create put new record into storage
func Create(ctx context.Context, conn dynamodbiface.DynamoDBAPI, tableName string, req *schema.ScheduledRequest) error {
	log.Printf("store request table_name=%s %s\n", tableName, req.ToString())
	av, err := dynamodbattribute.MarshalMap(req)
	if err != nil {
		return errors.Wrapf(err, "dynamodbattribute.MarshalMap req %s", req.ToString())
	}
	if _, err := conn.PutItem(&dynamodb.PutItemInput{
		Item:      av,
		TableName: aws.String(tableName),
	}); err != nil {
		return errors.Wrapf(err, "conn.PutItem req %s table_name=%s", req.ToString(), tableName)
	}
	return nil
}

// Get retrieve record from storage
func Get(ctx context.Context, conn dynamodbiface.DynamoDBAPI, tableName, reqID string) (*schema.ScheduledRequest, error) {
	log.Printf("get request table_name=%s id=%s\n", tableName, reqID)
	input := &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"ID": {
				S: aws.String(reqID),
			},
		},
	}
	output, err := conn.GetItem(input)
	if err != nil {
		return nil, errors.Wrapf(err, "conn.GetItem table_name=%s id=%s", tableName, reqID)
	}
	req := new(schema.ScheduledRequest)
	if err = dynamodbattribute.UnmarshalMap(output.Item, req); err != nil {
		return nil, errors.Wrapf(err, "dynamodbattribute.UnmarshalMap table_name=%s output=%s", tableName, output.GoString())
	}
	return req, nil
}

func updateResult(ctx context.Context, conn dynamodbiface.DynamoDBAPI, tableName, reqID string, resp *schema.Response, current time.Time) error {
	log.Printf("store execution result table_name=%s id=%s %s\n", tableName, reqID, resp.ToString())
	serialized, err := json.Marshal(resp)
	if err != nil {
		return errors.Wrapf(err, "json.Marshal resp %s", resp.ToString())
	}
	result := string(serialized)
	if _, err = conn.UpdateItem(&dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"ID": {
				S: aws.String(reqID),
			},
		},
		UpdateExpression: aws.String("SET ExecutionResult = :r, ExecutedAt = :e"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":r": {
				S: aws.String(result),
			},
			":e": {
				S: aws.String(current.Format(unixFormat)),
			},
		},
	}); err != nil {
		return errors.Wrapf(err, "conn.UpdateItem id=%s table_name=%s result=%s", reqID, tableName, result)
	}
	return nil
}

func removeRequest(ctx context.Context, conn dynamodbiface.DynamoDBAPI, tableName, reqID string) error {
	log.Printf("remove request table_name=%s id=%s\n", tableName, reqID)
	if _, err := conn.DeleteItem(&dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"ID": {
				S: aws.String(reqID),
			},
		},
	}); err != nil {
		return errors.Wrapf(err, "conn.DeleteItem id=%s table_name=%s", reqID, tableName)
	}
	return nil
}

func logFailure(ctx context.Context, conn dynamodbiface.DynamoDBAPI, tableName, reqID string, lerr error) error {
	log.Printf("log execution failure result table_name=%s id=%s \n", tableName, reqID)
	failure := lerr.Error()
	if _, err := conn.UpdateItem(&dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"ID": {
				S: aws.String(reqID),
			},
		},
		UpdateExpression: aws.String("SET FailureReason = :f"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":f": {
				S: aws.String(failure),
			},
		},
	}); err != nil {
		return errors.Wrapf(err, "conn.UpdateItem id=%s table_name=%s failure_reason=%s", reqID, tableName, failure)
	}
	return nil
}

// Lock set record Locking=true
func Lock(ctx context.Context, conn dynamodbiface.DynamoDBAPI, tableName, reqID string) error {
	return setLocking(ctx, conn, tableName, reqID, true)
}

func setLocking(ctx context.Context, conn dynamodbiface.DynamoDBAPI, tableName, reqID string, status bool) error {
	log.Printf("setLocking record table_name=%s id=%s status=%t \n", tableName, reqID, status)
	if _, err := conn.UpdateItem(&dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"ID": {
				S: aws.String(reqID),
			},
		},
		UpdateExpression: aws.String("SET Locking = :l"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":l": {
				BOOL: aws.Bool(status),
			},
		},
	}); err != nil {
		return errors.Wrapf(err, "conn.UpdateItem id=%s table_name=%s", reqID, tableName)
	}
	return nil

}

// Unlock set record Locking=false
func Unlock(ctx context.Context, conn dynamodbiface.DynamoDBAPI, tableName, reqID string) error {
	return setLocking(ctx, conn, tableName, reqID, false)
}
