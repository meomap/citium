// Package main provides cli commands to perform basic adminstrative operations on ScheduledRequest record
//
// To override default credentials with variables from the environment:
// export AWS_REGION=YOUR_REGION
// export AWS_ACCESS_KEY_ID=YOUR_AKID
// export AWS_SECRET_ACCESS_KEY=YOUR_SECRET_KEY
//
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"

	"github.com/meomap/citium/scheduler"
	"github.com/meomap/citium/schema"
)

func main() {
	var (
		action = flag.String("action", "", `command action name. the available options are:
	- create: request to add new record with specific parameters
	- get: retrieve scheduled request by given id
	- list: fetch all the scheduled requests to be run next
	- lock: request to lock record by given id
	- unlock: request to unlock record by given id
`)
		id            = flag.String("id", "", "request unique id")
		table         = flag.String("table", "", "dynamodb table to store request")
		freezeDur     = flag.Duration("freeze", time.Hour, "freeze duration (in secs) until effective date to execute request")
		method        = flag.String("method", http.MethodGet, "request method name")
		rURL          = flag.String("url", "", "request url path, could be absolute path or relative (in case BASE_URL env variable is set)")
		payload       = flag.String("payload", "", "payload data")
		headers       = flag.String("headers", "", "comma separated list of headers in format key:value")
		persistEnable = flag.Bool("persistent", false, "if true then persistently store request after execution")
	)
	flag.Parse()

	if *table == "" {
		fmt.Printf("Empty value of the required flag `-table`\n")
		os.Exit(1)
	}

	svc := dynamodb.New(session.Must(session.NewSession(nil)), aws.NewConfig())

	switch *action {
	case "list":
		records, err := scheduler.FetchSchedRequests(context.Background(), svc, *table, time.Now().UTC())
		if err != nil {
			panic(err)
		}
		serialized, err := json.Marshal(records)
		if err != nil {
			panic(err)
		}
		fmt.Println(string(serialized))
	case "create":
		req := &schema.ScheduledRequest{
			ID:              *id,
			CreatedAt:       time.Now().UTC(),
			Method:          *method,
			URL:             *rURL,
			Payload:         *payload,
			PersistentStore: *persistEnable,
		}
		if *headers != "" {
			req.Headers = map[string]string{}
			lst := strings.Split(*headers, ",")
			for _, v := range lst {
				parts := strings.Split(v, ":")
				req.Headers[parts[0]] = parts[1]
			}
		}
		req.EffectiveAfter = req.CreatedAt.Add(*freezeDur)
		valid, err := govalidator.ValidateStruct(req)
		if err != nil {
			panic(err)
		} else if !valid {
			panic("Request validation still failed somehow")
		}
		if err = scheduler.Create(context.Background(), svc, *table, req); err != nil {
			panic(err)
		}
	case "get":
		req, err := scheduler.Get(context.Background(), svc, *table, *id)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				if aerr.Code() == dynamodb.ErrCodeResourceNotFoundException {
					fmt.Println("not found")
					return
				}
			}
			panic(err)
		}
		serialized, err := json.Marshal(req)
		if err != nil {
			panic(err)
		}
		fmt.Println(string(serialized))
	case "lock":
		if err := scheduler.Lock(context.Background(), svc, *table, *id); err != nil {
			panic(err)
		}
	case "unlock":
		if err := scheduler.Unlock(context.Background(), svc, *table, *id); err != nil {
			panic(err)
		}
	default:
		flag.PrintDefaults()
		os.Exit(1)
	}
}
