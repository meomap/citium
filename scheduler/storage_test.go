package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meomap/citium/schema"
)

type mockDynamoDB struct {
	dynamodbiface.DynamoDBAPI
	once *sync.Once
	mu   *sync.Mutex
	// scan function
	lastScanQ string
	items     []map[string]*dynamodb.AttributeValue
	scanErr   error
	// get function
	lastGetQ string
	item     map[string]*dynamodb.AttributeValue
	getErr   error
	// put function
	lastPutItem *dynamodb.PutItemInput
	putErr      error
	// update function
	lastUpdateItem *dynamodb.UpdateItemInput
	updateErr      error
	// delete function
	lastDeleteItem *dynamodb.DeleteItemInput
	delErr         error
}

func (mdb *mockDynamoDB) clear() {
	mdb.once = new(sync.Once)
	mdb.mu = new(sync.Mutex)
	mdb.items = []map[string]*dynamodb.AttributeValue{}
	mdb.lastScanQ = ""
	mdb.scanErr = nil
	mdb.lastPutItem = nil
	mdb.putErr = nil
	mdb.lastUpdateItem = nil
	mdb.updateErr = nil
	mdb.item = map[string]*dynamodb.AttributeValue{}
	mdb.lastGetQ = ""
	mdb.getErr = nil
}

func (mdb *mockDynamoDB) Scan(input *dynamodb.ScanInput) (*dynamodb.ScanOutput, error) {
	mdb.lastScanQ = input.GoString()
	if mdb.scanErr != nil {
		return nil, mdb.scanErr
	}
	return &dynamodb.ScanOutput{
		ScannedCount: aws.Int64(int64(len(mdb.items))),
		Items:        mdb.items,
	}, nil
}

func (mdb *mockDynamoDB) GetItem(input *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
	mdb.lastGetQ = input.GoString()
	if mdb.getErr != nil {
		return nil, mdb.getErr
	}
	return &dynamodb.GetItemOutput{
		Item: mdb.item,
	}, nil
}

func (mdb *mockDynamoDB) PutItem(input *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
	mdb.lastPutItem = input
	if mdb.putErr != nil {
		return nil, mdb.putErr
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (mdb *mockDynamoDB) DeleteItem(input *dynamodb.DeleteItemInput) (*dynamodb.DeleteItemOutput, error) {
	mdb.mu.Lock()
	mdb.lastDeleteItem = input
	mdb.mu.Unlock()
	if mdb.delErr != nil {
		return nil, mdb.delErr
	}
	return &dynamodb.DeleteItemOutput{}, nil
}

func (mdb *mockDynamoDB) UpdateItem(input *dynamodb.UpdateItemInput) (*dynamodb.UpdateItemOutput, error) {
	mdb.mu.Lock()
	mdb.lastUpdateItem = input
	mdb.mu.Unlock()
	var err error
	mdb.once.Do(func() {
		err = mdb.updateErr
	})
	if err != nil {
		return nil, mdb.updateErr
	}
	return &dynamodb.UpdateItemOutput{}, nil
}

func TestFetchSchedRequests(t *testing.T) {
	mockConn := new(mockDynamoDB)
	table := "FetchSchedRequests_test"
	for _, c := range []struct {
		caseName string
		setup    func()
		err      bool
		wantLen  int
	}{
		{
			caseName: "empty",
			setup:    func() {},
			wantLen:  0,
		},
		{
			caseName: "single_record",
			setup: func() {
				mockConn.items = []map[string]*dynamodb.AttributeValue{
					{
						"ID":             {S: aws.String("test-single-record")},
						"CreatedAt":      {S: aws.String("2018-09-01T00:02:03Z")},
						"EffectiveAfter": {S: aws.String("2018-09-02T00:02:03Z")},
					},
				}
			},
			wantLen: 1,
		},
		{
			caseName: "multi_records",
			setup: func() {
				mockConn.items = []map[string]*dynamodb.AttributeValue{
					{
						"ID":             {S: aws.String("test-multiple-records-1")},
						"EffectiveAfter": {S: aws.String("2018-09-02T00:02:03Z")},
					},
					{
						"ID":             {S: aws.String("test-multiple-records-2")},
						"EffectiveAfter": {S: aws.String("2018-09-03T00:02:03Z")},
					},
					{
						"ID":             {S: aws.String("test-multiple-records-3")},
						"EffectiveAfter": {S: aws.String("2018-09-04T00:02:03Z")},
					},
				}
			},
			wantLen: 3,
		},
		{
			caseName: "scan_error",
			setup: func() {
				mockConn.scanErr = errors.New("internal error")
			},
			err: true,
		},
	} {
		t.Run(fmt.Sprintf("case=%s", c.caseName), func(t *testing.T) {
			mockConn.clear()
			c.setup()
			current := time.Now().UTC()
			records, err := FetchSchedRequests(context.Background(), mockConn, table, current)
			if c.err == true {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				lenRecords := len(records)
				assert.Equal(t, c.wantLen, lenRecords)
				// must scan with date time in ISO format
				assert.Contains(t, mockConn.lastScanQ, current.Format(unixFormat))
				// to prevent duplicate data bug
				for i := 0; i < lenRecords-1; i++ {
					assert.NotEqual(t, records[i].ID, records[i+1].ID)
				}
			}
		})
	}
}

func TestCreateRequest(t *testing.T) {
	mockConn := new(mockDynamoDB)
	table := "create_test"
	req := &schema.ScheduledRequest{
		ID:             "test-create",
		CreatedAt:      time.Now().UTC(),
		EffectiveAfter: time.Now().Add(time.Hour).UTC(),
	}
	for _, c := range []struct {
		caseName string
		setup    func()
		err      bool
	}{
		{
			caseName: "ok",
			setup:    func() {},
		},
		{
			caseName: "error",
			setup: func() {
				mockConn.putErr = errors.New("internal error")
			},
			err: true,
		},
	} {
		t.Run(fmt.Sprintf("case=%s", c.caseName), func(t *testing.T) {
			mockConn.clear()
			c.setup()
			err := Create(context.Background(), mockConn, table, req)
			if c.err == true {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, mockConn.lastPutItem)
				assert.Equal(t, "test-create", *mockConn.lastPutItem.Item["ID"].S)
				assert.Equal(t, table, *mockConn.lastPutItem.TableName)
			}
		})
	}
}

func TestUpdateResult(t *testing.T) {
	mockConn := new(mockDynamoDB)
	table := "updateResult_test"
	req := &schema.ScheduledRequest{
		ID: "test-updateResult",
	}
	resp := &schema.Response{
		Code: http.StatusOK,
		Body: "Success",
	}
	seriallized := "{\"code\":200,\"body\":\"Success\"}"
	current := time.Now().UTC()
	for _, c := range []struct {
		caseName string
		setup    func()
		err      bool
	}{
		{
			caseName: "ok",
			setup:    func() {},
		},
		{
			caseName: "error",
			setup: func() {
				mockConn.updateErr = errors.New("internal error")
			},
			err: true,
		},
	} {
		t.Run(fmt.Sprintf("case=%s", c.caseName), func(t *testing.T) {
			mockConn.clear()
			c.setup()
			err := updateResult(context.Background(), mockConn, table, req.ID, resp, current)
			if c.err == true {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, mockConn.lastUpdateItem)
				assert.Equal(t, "test-updateResult", *mockConn.lastUpdateItem.Key["ID"].S)
				assert.Equal(t, seriallized, *mockConn.lastUpdateItem.ExpressionAttributeValues[":r"].S)
			}
		})
	}
}

func TestRemoveRequest(t *testing.T) {
	mockConn := new(mockDynamoDB)
	table := "removeRequest_test"
	req := &schema.ScheduledRequest{
		ID: "test-removeRequest",
	}
	for _, c := range []struct {
		caseName string
		setup    func()
		err      bool
	}{
		{
			caseName: "ok",
			setup:    func() {},
		},
		{
			caseName: "error",
			setup: func() {
				mockConn.delErr = errors.New("internal error")
			},
			err: true,
		},
	} {
		t.Run(fmt.Sprintf("case=%s", c.caseName), func(t *testing.T) {
			mockConn.clear()
			c.setup()
			err := removeRequest(context.Background(), mockConn, table, req.ID)
			if c.err == true {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, mockConn.lastDeleteItem)
				assert.Equal(t, req.ID, *mockConn.lastDeleteItem.Key["ID"].S)
				assert.Equal(t, table, *mockConn.lastDeleteItem.TableName)
			}
		})
	}
}

func TestLogFailure(t *testing.T) {
	mockConn := new(mockDynamoDB)
	table := "logFailure_test"
	req := &schema.ScheduledRequest{
		ID: "test-logFailure",
	}
	lerr := errors.New("Unexpected error happened!")
	for _, c := range []struct {
		caseName string
		setup    func()
		err      bool
	}{
		{
			caseName: "ok",
			setup:    func() {},
		},
		{
			caseName: "error",
			setup: func() {
				mockConn.updateErr = errors.New("internal error")
			},
			err: true,
		},
	} {
		t.Run(fmt.Sprintf("case=%s", c.caseName), func(t *testing.T) {
			mockConn.clear()
			c.setup()
			err := logFailure(context.Background(), mockConn, table, req.ID, lerr)
			if c.err == true {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, mockConn.lastUpdateItem)
				assert.Equal(t, "test-logFailure", *mockConn.lastUpdateItem.Key["ID"].S)
				assert.Equal(t, lerr.Error(), *mockConn.lastUpdateItem.ExpressionAttributeValues[":f"].S)
			}
		})
	}
}

func TestLockUnlock(t *testing.T) {
	mockConn := new(mockDynamoDB)
	table := "lock_unlock_test"
	req := &schema.ScheduledRequest{
		ID: "test-lock",
	}
	ctx := context.Background()
	for _, c := range []struct {
		caseName         string
		setup            func() error
		expectLockStatus bool
		err              bool
	}{
		{
			caseName: "lock-ok",
			setup: func() error {
				return Lock(ctx, mockConn, table, req.ID)
			},
			expectLockStatus: true,
		},
		{
			caseName: "lock-error",
			setup: func() error {
				mockConn.updateErr = errors.New("internal error")
				return Lock(ctx, mockConn, table, req.ID)
			},
			err: true,
		},
		{
			caseName: "unlock-ok",
			setup: func() error {
				return Unlock(ctx, mockConn, table, req.ID)
			},
			expectLockStatus: false,
		},
		{
			caseName: "unlock-error",
			setup: func() error {
				mockConn.updateErr = errors.New("internal error")
				return Unlock(ctx, mockConn, table, req.ID)
			},
			err: true,
		},
	} {
		t.Run(fmt.Sprintf("case=%s", c.caseName), func(t *testing.T) {
			mockConn.clear()
			err := c.setup()
			if c.err == true {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, mockConn.lastUpdateItem)
				assert.Equal(t, "test-lock", *mockConn.lastUpdateItem.Key["ID"].S)
				assert.Equal(t, c.expectLockStatus, *mockConn.lastUpdateItem.ExpressionAttributeValues[":l"].BOOL)
			}
		})
	}
}

func TestGetRequest(t *testing.T) {
	mockConn := new(mockDynamoDB)
	table := "get_test"
	reqID := "test-get-request-id"
	for _, c := range []struct {
		caseName string
		setup    func()
		err      bool
		want     schema.ScheduledRequest
	}{
		{
			caseName: "error_not_exist",
			setup: func() {
				mockConn.getErr = errors.New(dynamodb.ErrCodeResourceNotFoundException)
			},
			err: true,
		},
		{
			caseName: "ok",
			setup: func() {
				mockConn.item = map[string]*dynamodb.AttributeValue{
					"ID":             {S: aws.String("test-get-request-id")},
					"CreatedAt":      {S: aws.String("2018-09-01T00:02:03Z")},
					"EffectiveAfter": {S: aws.String("2018-09-02T00:02:03Z")},
				}
			},
			want: schema.ScheduledRequest{
				ID:             "test-get-request-id",
				CreatedAt:      time.Date(2018, time.September, 01, 0, 2, 3, 0, time.UTC),
				EffectiveAfter: time.Date(2018, time.September, 02, 0, 2, 3, 0, time.UTC),
			},
		},
	} {
		t.Run(fmt.Sprintf("case=%s", c.caseName), func(t *testing.T) {
			mockConn.clear()
			c.setup()
			record, err := Get(context.Background(), mockConn, table, reqID)
			if c.err == true {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, record)
				assert.Equal(t, c.want, *record)
			}
		})
	}
}
