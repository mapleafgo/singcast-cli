package translator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetectCountry_Override(t *testing.T) {
	got := DetectCountry("US")
	if got != "US" {
		t.Errorf("DetectCountry(\"US\") = %q, want \"US\"", got)
	}
	got = DetectCountry("jp")
	if got != "JP" {
		t.Errorf("DetectCountry(\"jp\") = %q, want \"JP\"", got)
	}
	got = DetectCountry("  CN  ")
	if got != "CN" {
		t.Errorf("DetectCountry(\"  CN  \") = %q, want \"CN\"", got)
	}
}

func TestDetectCountryWithFallback_Override(t *testing.T) {
	cc, fallback := DetectCountryWithFallback("  CN  ")
	if cc != "CN" || fallback {
		t.Errorf("DetectCountryWithFallback(\"  CN  \") = %q, fallback=%v, want CN, false", cc, fallback)
	}
}

func TestTranslateFromConfigFallsBackOnNonLetterCountryOverride(t *testing.T) {
	out, _, _, err := translateFromConfig(&RawConfig{}, &Options{Country: "1X"})
	if err != nil {
		t.Fatalf("translateFromConfig() error = %v", err)
	}
	if strings.Contains(out, "geoip-1x") || strings.Contains(out, ".1x") {
		t.Errorf("non-letter country override leaked into rules: %s", out)
	}
}

func TestDetectCountry_FallbackOnEmpty(t *testing.T) {
	got := DetectCountry("")
	if len(got) != 2 {
		t.Errorf("DetectCountry(\"\") = %q, want 2-letter code", got)
	}
}

func TestDetectCountryByIP_ParseResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"1.2.3.4","country":"JP"}`))
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result struct {
		Country string `json:"country"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Country != "JP" {
		t.Errorf("country = %q, want JP", result.Country)
	}
}
