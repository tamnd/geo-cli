package geo

import (
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions.
// The client's HTTP behaviour is covered in geo_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "geo" {
		t.Errorf("Scheme = %q, want geo", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "geo" {
		t.Errorf("Identity.Binary = %q, want geo", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	typ, id, err := Domain{}.Classify("200300943")
	if err != nil {
		t.Fatalf("Classify error: %v", err)
	}
	if typ != "dataset" {
		t.Errorf("type = %q, want dataset", typ)
	}
	if id != "200300943" {
		t.Errorf("id = %q, want 200300943", id)
	}
}

func TestClassifyGSE(t *testing.T) {
	cases := []struct {
		in  string
		id  string
	}{
		{"GSE300943", "GSE300943"},
		{"GDS1234", "GDS1234"},
		{"GPL24676", "GPL24676"},
		{"GSM5678901", "GSM5678901"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil {
			t.Errorf("Classify(%q) error: %v", tc.in, err)
			continue
		}
		if typ != "dataset" {
			t.Errorf("Classify(%q) type = %q, want dataset", tc.in, typ)
		}
		if id != tc.id {
			t.Errorf("Classify(%q) id = %q, want %q", tc.in, id, tc.id)
		}
	}
}

func TestClassifyInvalid(t *testing.T) {
	cases := []string{"RNA-seq", "cancer study", "homo sapiens", ""}
	for _, tc := range cases {
		_, _, err := Domain{}.Classify(tc)
		if err == nil {
			t.Errorf("Classify(%q): expected error, got nil", tc)
		}
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		typ  string
		id   string
		want string
	}{
		{"dataset", "200300943", "https://www.ncbi.nlm.nih.gov/geo/query/acc.cgi?acc=200300943"},
		{"dataset", "GSE300943", "https://www.ncbi.nlm.nih.gov/geo/query/acc.cgi?acc=GSE300943"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.typ, tc.id)
		if err != nil {
			t.Errorf("Locate(%q, %q) error: %v", tc.typ, tc.id, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Locate(%q, %q) = %q, want %q", tc.typ, tc.id, got, tc.want)
		}
	}
}

func TestLocateInvalidType(t *testing.T) {
	_, err := Domain{}.Locate("page", "something")
	if err == nil {
		t.Error("expected error for unknown type")
	}
}
