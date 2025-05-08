package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	backoff "github.com/cenkalti/backoff/v5"
)

// DoWithRetry sends req, retrying on 429.
// It looks for Retry-After, otherwise uses your backoff policy,
// and always respects ctx cancellation.
func DoWithRetry(ctx context.Context, client *http.Client, req *http.Request, b backoff.BackOff) (*http.Response, error) {
	// 1) Buffer body so we can replay it:
	var getBody func() (io.ReadCloser, error)
	if req.GetBody != nil {
		getBody = req.GetBody
	} else if req.Body != nil {
		buf, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("buffering body: %w", err)
		}
		req.Body.Close()
		getBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(buf)), nil
		}
	}

	for {
		// 2) Clone request (with new body) and your new ctx:
		attempt, err := cloneRequest(ctx, req, getBody)
		if err != nil {
			return nil, err
		}

		resp, err := client.Do(attempt)
		if err != nil {
			return nil, err
		}

		// 3) If not 429, return immediately:
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}

		// 4) Got 429 → figure wait time:
		wait := parseRetryAfter(resp.Header.Get("Retry-After"))
		resp.Body.Close()

		if wait > 0 {
			// honor Retry-After exactly
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		} else {
			// fall back to exponential/backoff policy
			next := b.NextBackOff()
			if next == backoff.Stop {
				return nil, fmt.Errorf("retry limit reached")
			}
			select {
			case <-time.After(next):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		// loop → try again
	}
}

func cloneRequest(ctx context.Context, r *http.Request, getBody func() (io.ReadCloser, error)) (*http.Request, error) {
	nr := r.Clone(ctx)
	if getBody != nil {
		body, err := getBody()
		if err != nil {
			return nil, err
		}
		nr.Body = body
		nr.GetBody = getBody
		nr.ContentLength = r.ContentLength
	}
	return nr, nil
}

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	// “Retry-After: 120”
	if secs, err := strconv.Atoi(header); err == nil {
		return time.Duration(secs) * time.Second
	}
	// “Retry-After: Wed, 21 Oct 2015 07:28:00 GMT”
	if t, err := http.ParseTime(header); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
