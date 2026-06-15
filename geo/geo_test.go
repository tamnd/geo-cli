package geo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testServer(t *testing.T, mux *http.ServeMux) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 0
	return srv, NewClient(cfg)
}

func TestSearch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/esearch.fcgi", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("db") != "gds" {
			http.Error(w, "wrong db", 400)
			return
		}
		json.NewEncoder(w).Encode(wireSearch{
			ESearchResult: struct {
				Count  string   `json:"count"`
				IDList []string `json:"idlist"`
			}{
				Count:  "1715951",
				IDList: []string{"200300943", "200331505"},
			},
		})
	})
	_, client := testServer(t, mux)
	ids, total, err := client.Search(context.Background(), "RNA-seq cancer", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1715951 {
		t.Errorf("total = %d, want 1715951", total)
	}
	if len(ids) != 2 {
		t.Errorf("len = %d, want 2", len(ids))
	}
	if ids[0] != "200300943" {
		t.Errorf("ids[0] = %q, want 200300943", ids[0])
	}
}

func TestFetchDatasets(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/esummary.fcgi", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("db") != "gds" {
			http.Error(w, "wrong db", 400)
			return
		}
		d := wireDataset{
			UID:           "200300943",
			Accession:     "GSE300943",
			GDSType:       "Expression profiling by high throughput sequencing",
			Title:         "RNA-seq of breast cancer cell lines",
			Summary:       "Gene expression analysis of breast cancer.",
			GPL:           "GPL24676",
			PlatformTitle: "Illumina NovaSeq 6000",
			Taxon:         "Homo sapiens",
			Samples:       "10",
			PubMedIDs:     []string{"38123456"},
			FTPLink:       "ftp://ftp.ncbi.nlm.nih.gov/geo/series/GSE300nnn/GSE300943/",
			EntryType:     "GSE",
			PDAT:          "2025/01/15",
		}
		dBytes, _ := json.Marshal(d)
		result := map[string]json.RawMessage{
			"uids":      json.RawMessage(`["200300943"]`),
			"200300943": dBytes,
		}
		json.NewEncoder(w).Encode(map[string]any{"result": result})
	})
	_, client := testServer(t, mux)
	datasets, err := client.FetchDatasets(context.Background(), []string{"200300943"})
	if err != nil {
		t.Fatal(err)
	}
	if len(datasets) != 1 {
		t.Fatalf("len = %d, want 1", len(datasets))
	}
	d := datasets[0]
	if d.ID != "200300943" {
		t.Errorf("ID = %q, want 200300943", d.ID)
	}
	if d.Accession != "GSE300943" {
		t.Errorf("Accession = %q, want GSE300943", d.Accession)
	}
	if d.Organism != "Homo sapiens" {
		t.Errorf("Organism = %q, want Homo sapiens", d.Organism)
	}
	if d.SampleCount != "10" {
		t.Errorf("SampleCount = %q, want 10", d.SampleCount)
	}
	if len(d.PubMedIDs) != 1 || d.PubMedIDs[0] != "38123456" {
		t.Errorf("PubMedIDs = %v, want [38123456]", d.PubMedIDs)
	}
}

func TestFetchDatasetsNullPubMed(t *testing.T) {
	// PubMedIds may be null or [] in some records; verify we handle it cleanly.
	mux := http.NewServeMux()
	mux.HandleFunc("/esummary.fcgi", func(w http.ResponseWriter, r *http.Request) {
		raw := json.RawMessage(`{"uid":"100","Accession":"GSE100","title":"Test","PubMedIds":null}`)
		result := map[string]json.RawMessage{
			"uids": json.RawMessage(`["100"]`),
			"100":  raw,
		}
		json.NewEncoder(w).Encode(map[string]any{"result": result})
	})
	_, client := testServer(t, mux)
	datasets, err := client.FetchDatasets(context.Background(), []string{"100"})
	if err != nil {
		t.Fatal(err)
	}
	if len(datasets) != 1 {
		t.Fatalf("len = %d, want 1", len(datasets))
	}
	if datasets[0].PubMedIDs != nil {
		t.Errorf("PubMedIDs should be nil for null JSON, got %v", datasets[0].PubMedIDs)
	}
}

func TestGetDataset(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/esummary.fcgi", func(w http.ResponseWriter, r *http.Request) {
		d := wireDataset{UID: "200331505", Accession: "GSE331505", Title: "Single-cell RNA-seq"}
		dBytes, _ := json.Marshal(d)
		result := map[string]json.RawMessage{
			"uids":      json.RawMessage(`["200331505"]`),
			"200331505": dBytes,
		}
		json.NewEncoder(w).Encode(map[string]any{"result": result})
	})
	_, client := testServer(t, mux)
	d, err := client.GetDataset(context.Background(), "200331505")
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "200331505" {
		t.Errorf("ID = %q, want 200331505", d.ID)
	}
	if d.Accession != "GSE331505" {
		t.Errorf("Accession = %q, want GSE331505", d.Accession)
	}
}

func TestFetchDatasetsEmpty(t *testing.T) {
	_, client := testServer(t, http.NewServeMux())
	datasets, err := client.FetchDatasets(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if datasets != nil {
		t.Error("expected nil datasets for empty input")
	}
}
