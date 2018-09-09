package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meomap/citium/config"
	"github.com/meomap/citium/schema"
)

func TestNewClient(t *testing.T) {
	conf := &config.Configuration{
		BaseURL:   "test-baseurl",
		UserAgent: "test-useragent",
		Token:     "test-token",
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
				conf.BaseURL = "invalid-escape-%"
			},
			err: true,
		},
	} {
		t.Run(fmt.Sprintf("case=%s", c.caseName), func(t *testing.T) {
			c.setup()
			client, err := NewClient(conf)
			if c.err == true {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, client)
				assert.NotNil(t, client.baseURL)
			}
		})
	}
}

type mockSrv struct {
	mux *http.ServeMux
	srv *httptest.Server
}

func (ms *mockSrv) teardown(t *testing.T) {
	ms.srv.Close()
}

func setupMockSrv(t *testing.T) (*mockSrv, *HTTPClient) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	client, err := NewClient(&config.Configuration{
		BaseURL: srv.URL,
	})
	require.NoError(t, err)
	return &mockSrv{
		mux: mux,
		srv: srv,
	}, client
}

func TestExecRequest(t *testing.T) {
	mockSrv, client := setupMockSrv(t)
	defer mockSrv.teardown(t)
	req := new(schema.ScheduledRequest)
	mockURL := client.baseURL

	for _, c := range []struct {
		caseName    string
		description string
		setup       func()
		err         bool
		want        schema.Response
	}{
		{
			caseName:    "method_get_with_relative_url",
			description: "should pass with http status ok returned",
			setup: func() {
				req.Method = http.MethodGet
				req.URL = "test-get-with-relative-url"
				mockSrv.mux.HandleFunc("/test-get-with-relative-url", func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodGet, r.Method)
					w.WriteHeader(http.StatusOK)
				})
			},
			want: schema.Response{
				Code: http.StatusOK,
			},
		},
		{
			caseName:    "method_get_with_absolute_base_url",
			description: "should pass",
			setup: func() {
				client.baseURL = new(url.URL)
				req.Method = http.MethodGet
				req.URL = fmt.Sprintf("%s/test-get-with-absolute-url", mockSrv.srv.URL)
				mockSrv.mux.HandleFunc("/test-get-with-absolute-url", func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, http.MethodGet, r.Method)
					w.WriteHeader(http.StatusOK)
				})
			},
			want: schema.Response{
				Code: http.StatusOK,
			},
		},
		{
			caseName:    "method_get_with_user_agent_set",
			description: "should pass with User-Agent header",
			setup: func() {
				client.userAgent = "citium-v0.0.1"
				req.Method = http.MethodGet
				req.URL = "test-get-with-user-agent-header"
				mockSrv.mux.HandleFunc("/test-get-with-user-agent-header", func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "citium-v0.0.1", r.Header.Get("User-Agent"))
					w.WriteHeader(http.StatusOK)
				})
			},
			want: schema.Response{
				Code: http.StatusOK,
			},
		},
		{
			caseName:    "method_get_with_bearer_token",
			description: "should pass with Authorization header",
			setup: func() {
				client.token = "test-token"
				req.Method = http.MethodGet
				req.URL = "test-get-with-bearer-token"
				mockSrv.mux.HandleFunc("/test-get-with-bearer-token", func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
					w.WriteHeader(http.StatusOK)
				})
			},
			want: schema.Response{
				Code: http.StatusOK,
			},
		},
		{
			caseName:    "method_get_with_body_returned",
			description: "should pass with serialized response payload",
			setup: func() {
				req.Method = http.MethodGet
				req.Headers = map[string]string{
					"Content-Type": "application/json",
					"Accept":       "application/json",
				}
				req.URL = "test-get-with-body-returned"
				mockSrv.mux.HandleFunc("/test-get-with-body-returned", func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
					assert.Equal(t, "application/json", r.Header.Get("Accept"))
					w.WriteHeader(http.StatusOK)
					_, werr := w.Write([]byte("{\"message\":\"test-body-response\"}"))
					require.NoError(t, werr)
				})
			},
			want: schema.Response{
				Code: http.StatusOK,
				Body: "{\"message\":\"test-body-response\"}",
			},
		},
		{
			caseName:    "method_post_with_payload",
			description: "should pass",
			setup: func() {
				req.Method = http.MethodPost
				req.Headers = map[string]string{
					"Content-Type": "application/json",
					"Accept":       "application/json",
				}
				req.URL = "test-post-with-payload"
				req.Payload = "{\"data\":\"test-post-payload-data\"}"
				mockSrv.mux.HandleFunc("/test-post-with-payload", func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
					assert.Equal(t, "application/json", r.Header.Get("Accept"))
					w.WriteHeader(http.StatusCreated)
					_, werr := w.Write([]byte("{\"id\":\"test-post-payload-id\"}"))
					require.NoError(t, werr)
				})
			},
			want: schema.Response{
				Code: http.StatusCreated,
				Body: "{\"id\":\"test-post-payload-id\"}",
			},
		},
		{
			caseName:    "method_put_ok",
			description: "should pass",
			setup: func() {
				req.Method = http.MethodPut
				req.URL = "test-put-ok"
				mockSrv.mux.HandleFunc("/test-put-ok", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				})
			},
			want: schema.Response{
				Code: http.StatusOK,
			},
		},
		{
			caseName:    "method_delete_ok",
			description: "should pass",
			setup: func() {
				req.Method = http.MethodDelete
				req.URL = "test-delete-ok"
				mockSrv.mux.HandleFunc("/test-delete-ok", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				})
			},
			want: schema.Response{
				Code: http.StatusOK,
			},
		},
		{
			caseName:    "request_invalid_method",
			description: "should raise error",
			setup: func() {
				req.Method = "☹"
				req.URL = "test-invalid-method"
				mockSrv.mux.HandleFunc("/test-invalid-method", func(w http.ResponseWriter, r *http.Request) {
					assert.Fail(t, "should never reach server")
				})
			},
			err: true,
		},
		{
			caseName:    "request_invalid_url",
			description: "should raise error",
			setup: func() {
				req.URL = "test-invalid-url-%"
				mockSrv.mux.HandleFunc("/test-invalid-url-%", func(w http.ResponseWriter, r *http.Request) {
					assert.Fail(t, "should never reach server")
				})
			},
			err: true,
		},
		{
			caseName:    "request_url_not_found",
			description: "should pass with response status code 404",
			setup: func() {
				req.Method = http.MethodGet
				req.URL = "test-url-not-found"
			},
			want: schema.Response{
				Code: http.StatusNotFound,
				Body: "404 page not found\n",
			},
		},
	} {
		t.Run(fmt.Sprintf("case=%s/description=%s", c.caseName, c.description), func(t *testing.T) {
			// safeguard against this case `method_get_with_absolute_base_url` consequence
			client.baseURL = mockURL
			c.setup()
			resp, err := execRequest(context.Background(), client, req)
			if c.err == true {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp)
				assert.Equal(t, c.want, *resp)
			}
		})
	}
}

func TestMustNewClient(t *testing.T) {
	cli := Must(&HTTPClient{}, nil)
	assert.NotNil(t, cli)
	assert.Panics(t, func() {
		Must(nil, errors.New("Can't create new client"))
	})
}
