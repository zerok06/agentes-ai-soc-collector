package forwarder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/agentes-ai/qradar-collector/internal/models"
)

const (
	maxRetries     = 3
	initialBackoff = 1 * time.Second
)

// Forwarder sends transformed payloads to the external ingestion API.
type Forwarder struct {
	httpClient *http.Client
	url        string
	apiKey     string
	logger     *zap.SugaredLogger
}

// NewForwarder creates a new HTTP forwarder.
func NewForwarder(destURL, apiKey string, timeoutSec int, logger *zap.SugaredLogger) *Forwarder {
	return &Forwarder{
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		url:    destURL,
		apiKey: apiKey,
		logger: logger,
	}
}

// Send marshals the payload to JSON and POSTs it to the destination API.
// Retries with exponential backoff on 5xx responses.
func (f *Forwarder) Send(ctx context.Context, payload *models.OutputPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			f.logger.Warnw("retrying forward",
				"attempt", attempt,
				"backoff", backoff,
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.url, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("creating forward request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", f.apiKey)

		resp, err := f.httpClient.Do(req)
		if err != nil {
			if attempt < maxRetries {
				continue
			}
			return fmt.Errorf("executing forward request: %w", err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Retry on 5xx errors.
		if resp.StatusCode >= 500 && attempt < maxRetries {
			f.logger.Warnw("destination server error, will retry",
				"status", resp.StatusCode,
				"body", string(body),
			)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("destination API error: status=%d body=%s", resp.StatusCode, string(body))
		}

		f.logger.Debugw("payload forwarded successfully", "status", resp.StatusCode)
		return nil
	}

	return fmt.Errorf("forward failed after %d retries", maxRetries)
}
