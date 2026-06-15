package geo

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

func init() { kit.Register(Domain{}) }

// Domain is the GEO driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "geo",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "geo",
			Short:  "A command line for NCBI GEO genomic datasets.",
			Long: `A command line for NCBI GEO (Gene Expression Omnibus).

geo reads genomic dataset records from NCBI GEO, which archives functional
genomics data from high-throughput experiments including microarrays and
next-generation sequencing. No API key required. 1.7M+ dataset records indexed.`,
			Site: "https://www.ncbi.nlm.nih.gov/geo/",
			Repo: "https://github.com/tamnd/geo-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		Summary: "Search GEO datasets by keyword (--limit, --start)",
		Args:    []kit.Arg{{Name: "query", Help: "search query (e.g. RNA-seq cancer, BRCA1)"}}}, searchDatasets)

	kit.Handle(app, kit.OpMeta{Name: "dataset", Group: "read", Single: true,
		Summary: "Get a single GEO dataset by numeric UID", URIType: "dataset", Resolver: true,
		Args: []kit.Arg{{Name: "uid", Help: "GEO numeric UID (e.g. 200300943)"}}}, getDataset)

	kit.Handle(app, kit.OpMeta{Name: "series", Group: "read", List: true,
		Summary: "List GSE series datasets matching a query (--limit, --start)",
		Args:    []kit.Arg{{Name: "query", Help: "search query"}}}, seriesDatasets)

	kit.Handle(app, kit.OpMeta{Name: "organism", Group: "read", List: true,
		Summary: "List GEO datasets for an organism (--limit, --start)",
		Args:    []kit.Arg{{Name: "name", Help: "organism name (e.g. Homo sapiens, Mus musculus)"}}}, organismDatasets)
}

// newClient builds the GEO client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	return NewClient(c), nil
}

// --- inputs ---

type searchInput struct {
	Query  string  `kit:"arg"          help:"search query"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Start  int     `kit:"flag"         help:"offset for pagination"`
	Client *Client `kit:"inject"`
}

type datasetInput struct {
	UID    string  `kit:"arg"    help:"GEO numeric UID"`
	Client *Client `kit:"inject"`
}

type seriesInput struct {
	Query  string  `kit:"arg"          help:"search query"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Start  int     `kit:"flag"         help:"offset for pagination"`
	Client *Client `kit:"inject"`
}

type organismInput struct {
	Name   string  `kit:"arg"          help:"organism name"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Start  int     `kit:"flag"         help:"offset for pagination"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func searchDatasets(ctx context.Context, in searchInput, emit func(*Dataset) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	datasets, _, err := in.Client.SearchAndFetch(ctx, in.Query, limit, in.Start)
	if err != nil {
		return err
	}
	for _, d := range datasets {
		if err := emit(d); err != nil {
			return err
		}
	}
	return nil
}

func getDataset(ctx context.Context, in datasetInput, emit func(*Dataset) error) error {
	d, err := in.Client.GetDataset(ctx, in.UID)
	if err != nil {
		return err
	}
	return emit(d)
}

func seriesDatasets(ctx context.Context, in seriesInput, emit func(*Dataset) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	// Restrict to GSE series entries.
	query := fmt.Sprintf("%s AND GSE[ETYP]", in.Query)
	datasets, _, err := in.Client.SearchAndFetch(ctx, query, limit, in.Start)
	if err != nil {
		return err
	}
	for _, d := range datasets {
		if err := emit(d); err != nil {
			return err
		}
	}
	return nil
}

func organismDatasets(ctx context.Context, in organismInput, emit func(*Dataset) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	// Use the GEO organism qualifier.
	query := fmt.Sprintf("%s[organism]", in.Name)
	datasets, _, err := in.Client.SearchAndFetch(ctx, query, limit, in.Start)
	if err != nil {
		return err
	}
	for _, d := range datasets {
		if err := emit(d); err != nil {
			return err
		}
	}
	return nil
}

// Classify turns any accepted input into the canonical (type, id).
// All-digit strings are GEO numeric UIDs; GSE/GDS/GPL/GSM-prefixed strings
// are GEO accessions — both map to the "dataset" type.
func (Domain) Classify(input string) (string, string, error) {
	s := strings.TrimSpace(input)
	if len(s) == 0 {
		return "", "", errs.Usage("empty GEO reference")
	}
	if allDigits(s) {
		return "dataset", s, nil
	}
	upper := strings.ToUpper(s)
	for _, prefix := range []string{"GSE", "GDS", "GPL", "GSM"} {
		if strings.HasPrefix(upper, prefix) {
			return "dataset", s, nil
		}
	}
	return "", "", errs.Usage("unrecognized GEO reference %q: must be a numeric UID or GSE/GDS/GPL/GSM accession", s)
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(t, id string) (string, error) {
	switch t {
	case "dataset":
		return fmt.Sprintf("https://www.ncbi.nlm.nih.gov/geo/query/acc.cgi?acc=%s", id), nil
	default:
		return "", errs.Usage("geo has no resource type %q", t)
	}
}

// allDigits reports whether s is a non-empty string of ASCII digits.
func allDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
