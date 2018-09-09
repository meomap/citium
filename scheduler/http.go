package scheduler

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/pkg/errors"
	"go.uber.org/multierr"

	"github.com/meomap/citium/config"
	"github.com/meomap/citium/schema"
)

const jsonMIME = "application/json"

// Requester abstracts do request interface
type Requester interface {
	DoRequest(ctx context.Context, method, urlStr string, headers map[string]string, body string) (*schema.Response, error)
}

// HTTPClient manages http request communication
type HTTPClient struct {
	*http.Client
	baseURL   *url.URL
	userAgent string
	token     string
}

// NewClient returns initialized http client
func NewClient(conf *config.Configuration) (*HTTPClient, error) {
	baseURL, err := url.Parse(conf.BaseURL)
	if err != nil {
		return nil, errors.Wrapf(err, "url.Parse")
	}
	return &HTTPClient{
		Client:    http.DefaultClient,
		baseURL:   baseURL,
		userAgent: conf.UserAgent,
		token:     conf.Token,
	}, nil
}

// Must ensures http client is properly initialized
func Must(client *HTTPClient, err error) *HTTPClient {
	if err != nil {
		panic(err)
	}
	return client
}

// DoRequest performs http request call by given parameters
func (c *HTTPClient) DoRequest(ctx context.Context, method, urlStr string, headers map[string]string, body string) (*schema.Response, error) {
	rel, err := url.Parse(urlStr)
	if err != nil {
		return nil, errors.Wrapf(err, "url.Parse rawurl=%s", urlStr)
	}
	// method & url
	u := c.baseURL.ResolveReference(rel)
	buf := strings.NewReader(body)
	log.Printf("do method=%s url=%s \n", method, u.String())
	req, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, errors.Wrapf(err, "http.NewRequest method=%s url=%s", method, u.String())
	}
	// headers
	for k, v := range headers {
		req.Header.Add(k, v)
	}
	if c.userAgent != "" {
		req.Header.Add("User-Agent", c.userAgent)
	}
	if c.token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	}

	req = req.WithContext(ctx)
	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "c.Do")
	}
	defer func() {
		if rerr := resp.Body.Close(); rerr != nil {
			err = multierr.Append(err, rerr)
		}
	}()
	raw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "ioutil.ReadAll resp.Body")
	}
	return &schema.Response{
		Code: resp.StatusCode,
		Body: string(raw),
	}, nil
}

func execRequest(ctx context.Context, client Requester, req *schema.ScheduledRequest) (*schema.Response, error) {
	log.Printf("execute request %s \n", req.ToString())
	resp, err := client.DoRequest(ctx, req.Method, req.URL, req.Headers, req.Payload)
	if err != nil {
		return nil, errors.Wrapf(err, "client.DoRequest method=%s url=%s", req.Method, req.URL)
	}
	return resp, nil
}
