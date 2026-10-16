package metrics

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// testRouter mirrors the shape of the real API routes so the path label can be
// checked against route templates rather than request URLs.
func testRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/health", func(http.ResponseWriter, *http.Request) {})
	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/packages", func(r chi.Router) {
			r.Get("/", func(http.ResponseWriter, *http.Request) {})
			r.Get("/{name}", func(http.ResponseWriter, *http.Request) {})
			r.Get("/{name}/{version}", func(http.ResponseWriter, *http.Request) {})
			r.Get("/{name}/{version}/contracts/{contract}/abi", func(http.ResponseWriter, *http.Request) {})
		})
		r.Get("/deployments/{chainId}/{address}", func(http.ResponseWriter, *http.Request) {})
	})
	return r
}

// TestMain initializes metrics once; the collectors register globally and
// cannot be registered twice.
func TestMain(m *testing.M) {
	Init(true, "test")
	os.Exit(m.Run())
}

func TestPathLabelUsesRouteTemplate(t *testing.T) {
	router := testRouter()

	tests := []struct {
		url  string
		want string
	}{
		{"/health", "/health"},
		{"/api/v1/packages", "/api/v1/packages"},
		// With and without a trailing slash share one label.
		{"/api/v1/packages/", "/api/v1/packages"},
		{"/api/v1/packages/my-token-sdk", "/api/v1/packages/{name}"},
		{"/api/v1/packages/my-token-sdk/1.2.3", "/api/v1/packages/{name}/{version}"},
		{
			"/api/v1/packages/my-token-sdk/1.2.3/contracts/ERC20Vault/abi",
			"/api/v1/packages/{name}/{version}/contracts/{contract}/abi",
		},
		{"/api/v1/deployments/1/0xabc123", "/api/v1/deployments/{chainId}/{address}"},
		{"/no/such/route", unmatchedPath},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			before := requestCount(t, tt.want)
			router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tt.url, nil))
			if got := requestCount(t, tt.want); got != before+1 {
				t.Errorf("no request counted under path %q (count %v, was %v)", tt.want, got, before)
			}
		})
	}
}

// TestUnknownPathsShareOneLabel is the cardinality guarantee: arbitrary URLs
// must not each create their own series.
func TestUnknownPathsShareOneLabel(t *testing.T) {
	router := testRouter()

	before := pathLabelCount(t)
	for _, url := range []string{"/wp-admin", "/.env", "/api/v1/packages/a/b/c/d/e"} {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, url, nil))
	}
	if got := pathLabelCount(t); got > before+1 {
		t.Errorf("unknown URLs created %d new path labels, want at most 1", got-before)
	}
}

// requestCount returns the total requests counted for a path label.
func requestCount(t *testing.T, path string) float64 {
	t.Helper()
	var total float64
	for _, m := range requestMetrics(t) {
		for _, l := range m.GetLabel() {
			if l.GetName() == "path" && l.GetValue() == path {
				total += m.GetCounter().GetValue()
			}
		}
	}
	return total
}

// pathLabelCount returns how many distinct path label values exist.
func pathLabelCount(t *testing.T) int {
	t.Helper()
	seen := map[string]struct{}{}
	for _, m := range requestMetrics(t) {
		for _, l := range m.GetLabel() {
			if l.GetName() == "path" {
				seen[l.GetValue()] = struct{}{}
			}
		}
	}
	return len(seen)
}

func requestMetrics(t *testing.T) []*dto.Metric {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %v", err)
	}
	for _, f := range families {
		if f.GetName() == "http_requests_total" {
			return f.GetMetric()
		}
	}
	return nil
}
