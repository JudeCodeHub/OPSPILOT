package httpx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/git4jude/opspilot/services/internal/platform/logging"
	"github.com/git4jude/opspilot/services/internal/platform/metrics"
)

// Client calls another service over HTTP.
//
//   - Each call gets its own timeout via context.WithTimeout.
//   - The caller's X-Request-ID is propagated so one request can be traced
//     across services in the logs.
//   - upstream_requests_total{target,status} and
//     upstream_request_duration_seconds{target} are recorded.
//   - There are deliberately NO automatic retries: retries hide failing
//     dependencies and amplify load during incidents. Failures stay visible.
type Client struct {
	target  string // short name used in metrics and errors, e.g. "payment"
	baseURL string // e.g. "http://payment:8080"
	timeout time.Duration
	http    *http.Client
	metrics *metrics.Registry
}

// NewClient builds a client for one upstream service.
func NewClient(target, baseURL string, timeout time.Duration, m *metrics.Registry) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 32
	return &Client{
		target:  target,
		baseURL: strings.TrimRight(baseURL, "/"),
		timeout: timeout,
		// No Client.Timeout: the per-call context deadline is the single source of truth.
		http:    &http.Client{Transport: transport},
		metrics: m,
	}
}

// Response is a fully-read upstream response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// UpstreamError describes a call that produced no usable HTTP response.
type UpstreamError struct {
	Target  string
	Timeout bool // true if the per-call deadline expired
	Err     error
}

func (e *UpstreamError) Error() string {
	if e.Timeout {
		return fmt.Sprintf("%s: request timed out: %v", e.Target, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Target, e.Err)
}

func (e *UpstreamError) Unwrap() error { return e.Err }

// IsTimeout reports whether err is an upstream timeout.
func IsTimeout(err error) bool {
	var ue *UpstreamError
	return errors.As(err, &ue) && ue.Timeout
}

// Do sends a request to path (e.g. "/charges") and reads the whole response.
// A non-2xx status is NOT an error: the caller decides what it means. An error
// is returned only when no response was received (*UpstreamError).
func (c *Client) Do(ctx context.Context, method, path string, body []byte, header http.Header) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, &UpstreamError{Target: c.target, Err: err}
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if id := logging.RequestID(ctx); id != "" {
		req.Header.Set(HeaderRequestID, id)
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	var respBody []byte
	if err == nil {
		respBody, err = io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
	}
	c.metrics.UpstreamDuration.WithLabelValues(c.target).Observe(time.Since(start).Seconds())

	if err != nil {
		timeout := errors.Is(ctx.Err(), context.DeadlineExceeded) || isNetTimeout(err)
		status := "error"
		if timeout {
			status = "timeout"
		}
		c.metrics.UpstreamRequests.WithLabelValues(c.target, status).Inc()
		return nil, &UpstreamError{Target: c.target, Timeout: timeout, Err: err}
	}

	c.metrics.UpstreamRequests.WithLabelValues(c.target, strconv.Itoa(resp.StatusCode)).Inc()
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: respBody}, nil
}

func isNetTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
