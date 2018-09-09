package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meomap/citium/config"
	"github.com/meomap/citium/schema"
)

type mockHTTPClient struct {
	counter    uint32
	once       *sync.Once
	requestErr error
}

func (mc *mockHTTPClient) DoRequest(ctx context.Context, method, urlStr string, headers map[string]string, body string) (*schema.Response, error) {
	atomic.AddUint32(&mc.counter, 1)
	var err error
	mc.once.Do(func() {
		err = mc.requestErr
	})
	return &schema.Response{}, err
}

func (mc *mockHTTPClient) clear() {
	mc.counter = 0
	mc.once = new(sync.Once)
}

func (mc *mockHTTPClient) assertCalled(t *testing.T, expect uint32) {
	assert.Equal(t, expect, atomic.LoadUint32(&mc.counter))
}

func TestTriggerAPI(t *testing.T) {
	mockConn := new(mockDynamoDB)
	mockClient := new(mockHTTPClient)
	table := "TriggerAPI_test"
	conf := &config.Configuration{
		TableName: table,
	}
	for _, c := range []struct {
		caseName        string
		description     string
		setup           func()
		expectExecTimes uint32
		err             bool
	}{
		{
			caseName:    "empty",
			description: "should pass without executing any requests",
			setup:       func() {},
		},
		{
			caseName:    "multiple requests",
			description: "should pass with goroutines executed",
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
			expectExecTimes: 3,
		},
		{
			caseName:    "errors raised in middle of executing multiple requests",
			description: "should wait for all requests finished while collecting errors",
			setup: func() {
				mockConn.items = []map[string]*dynamodb.AttributeValue{
					{
						"ID":             {S: aws.String("test-multiple-records-4")},
						"EffectiveAfter": {S: aws.String("2018-09-02T00:02:03Z")},
					},
					{
						"ID":             {S: aws.String("test-multiple-records-5")},
						"EffectiveAfter": {S: aws.String("2018-09-03T00:02:03Z")},
					},
					{
						"ID":             {S: aws.String("test-multiple-records-6")},
						"EffectiveAfter": {S: aws.String("2018-09-04T00:02:03Z")},
					},
				}
				// locking setup failed for first request
				mockConn.updateErr = errors.New("Internal error")
			},
			expectExecTimes: 2,
			err:             true,
		},
		{
			caseName:    "errors due to request execution",
			description: "should failed with error",
			setup: func() {
				mockConn.items = []map[string]*dynamodb.AttributeValue{
					{
						"ID":             {S: aws.String("test-multiple-records-4")},
						"EffectiveAfter": {S: aws.String("2018-09-02T00:02:03Z")},
					},
				}
				mockClient.requestErr = errors.New("Request error")
			},
			expectExecTimes: 1,
			err:             true,
		},
		{
			caseName:    "errors due to remove request execution",
			description: "should failed with error",
			setup: func() {
				mockConn.items = []map[string]*dynamodb.AttributeValue{
					{
						"ID":             {S: aws.String("test-multiple-records-4")},
						"EffectiveAfter": {S: aws.String("2018-09-02T00:02:03Z")},
					},
				}
				mockConn.delErr = errors.New("Internal error")
			},
			expectExecTimes: 1,
			err:             true,
		},
	} {
		t.Run(fmt.Sprintf("case=%s/description=%s", c.caseName, c.description), func(t *testing.T) {
			mockConn.clear()
			mockClient.clear()
			c.setup()
			err := TriggerAPI(context.Background(), conf, mockConn, mockClient)
			if c.err == true {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			mockClient.assertCalled(t, c.expectExecTimes)
		})
	}
}
