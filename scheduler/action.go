package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/pkg/errors"
	"go.uber.org/multierr"

	"github.com/meomap/citium/config"
	"github.com/meomap/citium/schema"
)

// TriggerAPI executes the pre-scheduled rest API calls
func TriggerAPI(ctx context.Context, conf *config.Configuration, dbconn dynamodbiface.DynamoDBAPI, client Requester) error {
	requests, err := FetchSchedRequests(ctx, dbconn, conf.TableName, time.Now().UTC())
	if err != nil {
		return errors.Wrap(err, "fetchSchedRequests")
	}
	lenReqs := len(requests)

	var wg sync.WaitGroup

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		for i := 0; i < lenReqs; i++ {
			req := requests[i]
			wg.Add(1)
			go func() {
				defer wg.Done()
				if gErr := execute(ctx, dbconn, client, req, conf.TableName); gErr != nil {
					errc <- errors.Wrapf(gErr, "execute %s table_name=%s", req.ToString(), conf.TableName)
				}
			}()
		}
		wg.Wait()
	}()
	for gErr := range errc {
		if gErr != nil {
			err = multierr.Combine(err, gErr)
		}
	}
	// by default a scheduled function is invoke asynchronous thus it will be retried twice
	// when failure happened
	// https://docs.aws.amazon.com/lambda/latest/dg/invoking-lambda-function.html#supported-event-source-scheduled-events
	return err
}

func execute(ctx context.Context, dbconn dynamodbiface.DynamoDBAPI, client Requester, req *schema.ScheduledRequest, table string) error {
	// Always lock the request to be executing.
	// If execution succeeded and PersistentStore=true, it will not be scheduled at the next run.
	// In case execution failure, manual intervention is needed thus it should not be rolling out
	// next time also.
	err := Lock(ctx, dbconn, table, req.ID)
	if err != nil {
		return errors.Wrapf(err, "lock id=%s table_name=%s", req.ID, table)
	}

	resp, err := execRequest(ctx, client, req)
	if err != nil {
		err = errors.Wrapf(err, "execRequest %s", req.ToString())
		return multierr.Append(err, logFailure(ctx, dbconn, table, req.ID, err))
	}
	if req.PersistentStore {
		if err = updateResult(ctx, dbconn, table, req.ID, resp, time.Now().UTC()); err != nil {
			return errors.Wrapf(err, "storeResult req[%s] resp[%s]", req.ToString(), resp.ToString())
		}
	} else {
		if err = removeRequest(ctx, dbconn, table, req.ID); err != nil {
			return errors.Wrapf(err, "removeRequest %s", req.ToString())
		}
	}
	return nil
}
