package schema

import (
	"fmt"
	"time"
)

// ScheduledRequest defines the parameters for a request call triggering
type ScheduledRequest struct {
	// Unique ID across global region.
	ID string `json:"ID" valid:"required"`

	// Created datetime which will be seriallized into unix nano seconds since epoch.
	CreatedAt time.Time `json:"CreatedAt" valid:"required"`

	// This properties is updated after execution and PersistentStore=true
	ExecutedAt time.Time `json:"ExecutedAt"`

	// The attribute to specify the time point in which request should be triggered.
	// Note that this will not be the exact executing time as there will be slippering window
	// time due to configured polling interval and locking time.
	// EffectiveAfter time.Time `json:"EffectiveAfter" dynamodbav:",unixtime`
	EffectiveAfter time.Time `json:"EffectiveAfter" valid:"required"`

	// The attribute to prevent request got executed even if effective date already past.
	Locking bool `json:"Locking"`

	// Attribute to log failure reason for previous execution attempt
	FailureReason string `json:"FailureReason"`

	// Request method name. Available options are:
	// - GET
	// - POST
	// - PUT
	// - DELETE
	Method string `json:"Method" valid:"required,in(GET|PUT|POST|DELETE)"`

	// Absolute path or relative url string
	URL string `json:"URL" valid:"required"`

	// Request optional data payload
	Payload string `json:"Payload"`

	// Optional headers by specific request
	Headers map[string]string `json:"Headers"`

	// A boolean value that determines record persistency after the execution.
	// By default scheduled request will be removed after executing if this attribute is
	// not set.
	PersistentStore bool `json:"PersistentStore"`

	// A string that captures the output from the response returned, available only after
	// request got called and `PersistentStore=true`.
	ExecutionResult string `json:"ExecutionResult"`
}

// ToString returns string representation
func (req ScheduledRequest) ToString() string {
	return fmt.Sprintf("id=%s effective_after=%s locking=%t", req.ID, req.EffectiveAfter, req.Locking)
}

// Response capture the execution result
type Response struct {
	// HTTP status code
	Code int `json:"code"`
	// Response body data payload
	Body string `json:"body"`
}

// ToString returns string representation
func (resp Response) ToString() string {
	return fmt.Sprintf("code=%d body=%s", resp.Code, resp.Body)
}
