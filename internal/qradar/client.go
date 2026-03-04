package qradar

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	maxRetries     = 3
	initialBackoff = 1 * time.Second
	arielPollDelay = 2 * time.Second
	arielMaxPolls  = 60 // max polls before giving up (~2 minutes)
)

// Client interacts with the QRadar REST API.
type Client struct {
	httpClient *http.Client
	baseURL     string
	token       string
	version     string
	logger      *zap.SugaredLogger
	domainsMap  map[int64]string
	domainsMu   sync.RWMutex
}

// NewClient creates a new QRadar API client with connection pooling.
func NewClient(baseURL, token, version string, timeoutSec int, tlsInsecure bool, logger *zap.SugaredLogger) *Client {
	transport := &http.Transport{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: tlsInsecure,
		},
	}

	return &Client{
		httpClient: &http.Client{
			Timeout:   time.Duration(timeoutSec) * time.Second,
			Transport: transport,
		},
		baseURL: strings.TrimRight(baseURL, "/"),
		token:      token,
		version:    version,
		logger:     logger,
		domainsMap: make(map[int64]string),
	}
}

// GetDomainName fetches the human-readable name of a domain/tenant.
func (c *Client) GetDomainName(ctx context.Context, domainID int64) (string, error) {
	c.domainsMu.RLock()
	name, exists := c.domainsMap[domainID]
	c.domainsMu.RUnlock()

	if exists {
		return name, nil
	}

	endpoint := fmt.Sprintf("%s/config/domain_management/domains", c.baseURL)
	body, err := c.doRequestWithRetry(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("fetching domains: %w", err)
	}

	var domains []Domain
	if err := json.Unmarshal(body, &domains); err != nil {
		return "", fmt.Errorf("decoding domains: %w", err)
	}

	c.domainsMu.Lock()
	defer c.domainsMu.Unlock()
	for _, d := range domains {
		c.domainsMap[d.ID] = d.Name
	}

	if name, exists := c.domainsMap[domainID]; exists {
		return name, nil
	}

	return "Unknown Tenant", nil
}

// GetOffenses retrieves offenses updated after the given timestamp.
func (c *Client) GetOffenses(ctx context.Context, lastUpdatedTime int64) ([]Offense, error) {
	endpoint := fmt.Sprintf("%s/siem/offenses", c.baseURL)
	if lastUpdatedTime > 0 {
		endpoint = fmt.Sprintf("%s?filter=%s", endpoint,
			url.QueryEscape(fmt.Sprintf("last_updated_time>%d", lastUpdatedTime)))
	}

	c.logger.Debugw("fetching offenses", "endpoint", endpoint)

	body, err := c.doRequestWithRetry(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching offenses: %w", err)
	}

	var offenses []Offense
	if err := json.Unmarshal(body, &offenses); err != nil {
		return nil, fmt.Errorf("decoding offenses: %w", err)
	}

	c.logger.Infow("fetched offenses", "count", len(offenses))
	return offenses, nil
}

// GetOffense retrieves a single offense by ID.
func (c *Client) GetOffense(ctx context.Context, offenseID int64) (*Offense, error) {
	endpoint := fmt.Sprintf("%s/siem/offenses/%d", c.baseURL, offenseID)

	body, err := c.doRequestWithRetry(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching offense %d: %w", offenseID, err)
	}

	var offense Offense
	if err := json.Unmarshal(body, &offense); err != nil {
		return nil, fmt.Errorf("decoding offense %d: %w", offenseID, err)
	}

	return &offense, nil
}

// SearchEvents performs an Ariel search for events associated with an offense.
// It creates the search, polls until completion, and returns the results.
func (c *Client) SearchEvents(ctx context.Context, offenseID int64) ([]ArielEvent, error) {
	// Step 1: Create the Ariel search.
	aql := fmt.Sprintf(
		"SELECT sourceip, destinationip, username, "+
			"QIDNAME(qid) as eventname, "+
			"LOGSOURCENAME(logsourceid) as logsource, "+
			"UTF8(payload) as payload "+
			"FROM events WHERE INOFFENSE(%d) LIMIT 1 LAST 24 HOURS",
		offenseID,
	)

	searchEndpoint := fmt.Sprintf("%s/ariel/searches?query_expression=%s",
		c.baseURL, url.QueryEscape(aql))

	c.logger.Debugw("creating Ariel search", "offense_id", offenseID)

	body, err := c.doRequestWithRetry(ctx, http.MethodPost, searchEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Ariel search for offense %d: %w", offenseID, err)
	}

	var search ArielSearch
	if err := json.Unmarshal(body, &search); err != nil {
		return nil, fmt.Errorf("decoding Ariel search response: %w", err)
	}

	if search.SearchID == "" {
		return nil, fmt.Errorf("empty search ID returned for offense %d", offenseID)
	}

	// Step 2: Poll until search completes.
	statusEndpoint := fmt.Sprintf("%s/ariel/searches/%s", c.baseURL, search.SearchID)

	for i := 0; i < arielMaxPolls; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(arielPollDelay):
		}

		body, err := c.doRequestWithRetry(ctx, http.MethodGet, statusEndpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("polling Ariel search %s: %w", search.SearchID, err)
		}

		if err := json.Unmarshal(body, &search); err != nil {
			return nil, fmt.Errorf("decoding Ariel search status: %w", err)
		}

		c.logger.Debugw("Ariel search poll",
			"search_id", search.SearchID,
			"status", search.Status,
			"progress", search.Progress,
		)

		switch search.Status {
		case "COMPLETED":
			goto fetchResults
		case "ERROR":
			return nil, fmt.Errorf("Ariel search %s failed: %v", search.SearchID, search.ErrorMessages)
		case "CANCELED":
			return nil, fmt.Errorf("Ariel search %s was canceled", search.SearchID)
		}
	}

	return nil, fmt.Errorf("Ariel search %s timed out after %d polls", search.SearchID, arielMaxPolls)

fetchResults:
	// Step 3: Fetch results.
	resultsEndpoint := fmt.Sprintf("%s/ariel/searches/%s/results", c.baseURL, search.SearchID)
	body, err = c.doRequestWithRetry(ctx, http.MethodGet, resultsEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching Ariel results %s: %w", search.SearchID, err)
	}

	var result ArielResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding Ariel results: %w", err)
	}

	c.logger.Infow("Ariel search completed",
		"offense_id", offenseID,
		"search_id", search.SearchID,
		"event_count", len(result.Events),
	)

	return result.Events, nil
}

// doRequestWithRetry executes an HTTP request with exponential backoff retry.
func (c *Client) doRequestWithRetry(ctx context.Context, method, endpoint string, reqBody io.Reader) ([]byte, error) {
	backoff := initialBackoff

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			c.logger.Warnw("retrying request",
				"attempt", attempt,
				"backoff", backoff,
				"endpoint", endpoint,
			)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		req, err := http.NewRequestWithContext(ctx, method, endpoint, reqBody)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		// QRadar authentication headers.
		req.Header.Set("SEC", c.token)
		req.Header.Set("Version", c.version)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if attempt < maxRetries {
				continue
			}
			return nil, fmt.Errorf("executing request: %w", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		if readErr != nil {
			if attempt < maxRetries {
				continue
			}
			return nil, fmt.Errorf("reading response body: %w", readErr)
		}

		// Retry on 5xx errors.
		if resp.StatusCode >= 500 && attempt < maxRetries {
			c.logger.Warnw("server error, will retry",
				"status", resp.StatusCode,
				"endpoint", endpoint,
			)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(body))
		}

		return body, nil
	}

	return nil, fmt.Errorf("request failed after %d retries", maxRetries)
}
