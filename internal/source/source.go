// Package source fetches job postings from public job-board APIs.
package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Job is one posting, normalised across job boards.
type Job struct {
	ID          string // unique across sources, e.g. "greenhouse:druva:12345"
	Company     string
	Title       string
	Location    string
	URL         string
	Description string // plain text
	Posted      time.Time
}

// Company is one entry in companies.json.
type Company struct {
	Name string `json:"name"`
	ATS  string `json:"ats"`  // greenhouse | lever | ashby | remoteok
	Slug string `json:"slug"` // board name on that ATS
}

// Fetch returns all open jobs for a company.
func Fetch(ctx context.Context, c *http.Client, co Company) ([]Job, error) {
	switch co.ATS {
	case "greenhouse":
		return greenhouse(ctx, c, co)
	case "lever":
		return lever(ctx, c, co)
	case "ashby":
		return ashby(ctx, c, co)
	case "remoteok":
		return remoteOK(ctx, c, co)
	default:
		return nil, fmt.Errorf("unknown ats %q", co.ATS)
	}
}

// errRetry marks responses worth retrying (rate limits, server errors).
var errRetry = errors.New("retryable")

// getJSON GETs url and decodes the body into v, retrying with exponential
// backoff on network errors, 429 and 5xx.
func getJSON(ctx context.Context, c *http.Client, url string, v any) error {
	const attempts = 3
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			backoff := time.Duration(1<<i)*time.Second + time.Duration(rand.Intn(500))*time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		err = getOnce(ctx, c, url, v)
		if err == nil || !errors.Is(err, errRetry) {
			return err
		}
	}
	return err
}

func getOnce(ctx context.Context, c *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "job-radar/1.0 (+https://github.com/muhammedshamil8/job-radar)")
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", errRetry, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return fmt.Errorf("%w: %s", errRetry, resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("%w: %v", errRetry, err)
	}
	return json.Unmarshal(body, v)
}

var (
	tagRE   = regexp.MustCompile(`<[^>]*>`)
	spaceRE = regexp.MustCompile(`\s+`)
)

// plain turns (possibly entity-escaped) HTML into one line of text.
func plain(s string) string {
	s = html.UnescapeString(html.UnescapeString(s))
	s = tagRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(spaceRE.ReplaceAllString(s, " "))
}
