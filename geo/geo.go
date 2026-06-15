// Package geo is the library behind the geo command line:
// the HTTP client, request shaping, and the typed data models for NCBI GEO.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Host is the eUtils hostname this client talks to, and the host the URI
// driver in domain.go claims.
const Host = "eutils.ncbi.nlm.nih.gov"

const baseURL = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"

// Config holds the runtime settings for the GEO client.
type Config struct {
	BaseURL   string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns a Config with sensible defaults: 400ms rate limit,
// 3 retries, and a 30s timeout.
func DefaultConfig() Config {
	return Config{
		BaseURL:   baseURL,
		Rate:      400 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
		UserAgent: "geo-cli/0.1.0 (github.com/tamnd/geo-cli)",
	}
}

// Client talks to NCBI eUtils for GEO DataSets records.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client using the given Config.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

func (c *Client) wait() {
	if c.cfg.Rate > 0 {
		if since := time.Since(c.last); since < c.cfg.Rate {
			time.Sleep(c.cfg.Rate - since)
		}
	}
	c.last = time.Now()
}

func (c *Client) get(ctx context.Context, rawURL string, out any) error {
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			d := time.Duration(attempt) * 500 * time.Millisecond
			if d > 5*time.Second {
				d = 5 * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
		}
		c.wait()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", c.cfg.UserAgent)
		resp, err := c.http.Do(req)
		if err != nil {
			if attempt < c.cfg.Retries {
				continue
			}
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if attempt < c.cfg.Retries {
				continue
			}
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return fmt.Errorf("all retries exhausted")
}

// --- wire types (unexported) ---

type wireSearch struct {
	ESearchResult struct {
		Count  string   `json:"count"`
		IDList []string `json:"idlist"`
	} `json:"esearchresult"`
}

type wireDataset struct {
	UID           string   `json:"uid"`
	Accession     string   `json:"Accession"`
	GDSType       string   `json:"GDS_type"`
	Title         string   `json:"title"`
	Summary       string   `json:"summary"`
	GPL           string   `json:"GPL"`
	PlatformTitle string   `json:"GplatformTitle"`
	Taxon         string   `json:"taxon"`
	Samples       string   `json:"Samples"`
	PubMedIDs     []string `json:"PubMedIds"`
	FTPLink       string   `json:"FTPLink"`
	EntryType     string   `json:"entryType"`
	PDAT          string   `json:"PDAT"`
}

type wireSummary struct {
	Result map[string]json.RawMessage `json:"result"`
}

// --- public types ---

// Dataset is a single GEO DataSets record.
type Dataset struct {
	ID            string   `json:"id"                  kit:"id"`
	Accession     string   `json:"accession"`
	Title         string   `json:"title"`
	Summary       string   `json:"summary,omitempty"`
	Type          string   `json:"type,omitempty"`
	Platform      string   `json:"platform,omitempty"`
	PlatformTitle string   `json:"platform_title,omitempty"`
	Organism      string   `json:"organism,omitempty"`
	SampleCount   string   `json:"sample_count,omitempty"`
	PubMedIDs     []string `json:"pubmed_ids,omitempty"`
	EntryType     string   `json:"entry_type,omitempty"`
	Date          string   `json:"date,omitempty"`
	FTPLink       string   `json:"ftp_link,omitempty"`
}

func toDataset(w wireDataset) *Dataset {
	return &Dataset{
		ID:            w.UID,
		Accession:     w.Accession,
		Title:         w.Title,
		Summary:       w.Summary,
		Type:          w.GDSType,
		Platform:      w.GPL,
		PlatformTitle: w.PlatformTitle,
		Organism:      w.Taxon,
		SampleCount:   w.Samples,
		PubMedIDs:     w.PubMedIDs,
		EntryType:     w.EntryType,
		Date:          w.PDAT,
		FTPLink:       w.FTPLink,
	}
}

// Search searches GEO DataSets for records matching the query and returns IDs
// and the total hit count.
func (c *Client) Search(ctx context.Context, query string, limit, start int) ([]string, int, error) {
	u := fmt.Sprintf("%s/esearch.fcgi?db=gds&term=%s&retmax=%d&retstart=%d&retmode=json",
		c.cfg.BaseURL, url.QueryEscape(query), limit, start)
	var w wireSearch
	if err := c.get(ctx, u, &w); err != nil {
		return nil, 0, err
	}
	count := 0
	fmt.Sscanf(w.ESearchResult.Count, "%d", &count)
	return w.ESearchResult.IDList, count, nil
}

// FetchDatasets fetches dataset details for the given IDs (up to ~500 per
// call; callers should batch if needed).
func (c *Client) FetchDatasets(ctx context.Context, ids []string) ([]*Dataset, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	u := fmt.Sprintf("%s/esummary.fcgi?db=gds&id=%s&retmode=json",
		c.cfg.BaseURL, strings.Join(ids, ","))
	var w wireSummary
	if err := c.get(ctx, u, &w); err != nil {
		return nil, err
	}
	rawUIDs, ok := w.Result["uids"]
	if !ok {
		return nil, fmt.Errorf("no uids in esummary response")
	}
	var uids []string
	if err := json.Unmarshal(rawUIDs, &uids); err != nil {
		return nil, err
	}
	var datasets []*Dataset
	for _, uid := range uids {
		raw, ok := w.Result[uid]
		if !ok {
			continue
		}
		var wd wireDataset
		if err := json.Unmarshal(raw, &wd); err != nil {
			continue
		}
		datasets = append(datasets, toDataset(wd))
	}
	return datasets, nil
}

// GetDataset fetches a single dataset by its GEO numeric UID.
func (c *Client) GetDataset(ctx context.Context, uid string) (*Dataset, error) {
	datasets, err := c.FetchDatasets(ctx, []string{uid})
	if err != nil {
		return nil, err
	}
	if len(datasets) == 0 {
		return nil, fmt.Errorf("dataset %s not found", uid)
	}
	return datasets[0], nil
}

// SearchAndFetch searches GEO DataSets and returns full Dataset records.
func (c *Client) SearchAndFetch(ctx context.Context, query string, limit, start int) ([]*Dataset, int, error) {
	ids, total, err := c.Search(ctx, query, limit, start)
	if err != nil {
		return nil, 0, err
	}
	if len(ids) == 0 {
		return nil, total, nil
	}
	datasets, err := c.FetchDatasets(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	return datasets, total, nil
}
