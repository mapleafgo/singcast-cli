package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	cachedCountry     string
	cachedCountryOnce sync.Once
)

// DetectCountry determines the user's two-letter country code (ISO 3166-1 alpha-2).
// Priority: explicit argument > cached IP geolocation > fallback "CN".
func DetectCountry(override string) string {
	if cc := strings.TrimSpace(override); len(cc) == 2 {
		return strings.ToUpper(cc)
	}
	cachedCountryOnce.Do(func() {
		if cc, err := detectCountryByIP(); err == nil && len(cc) == 2 {
			cachedCountry = strings.ToUpper(cc)
		} else {
			cachedCountry = "CN"
		}
	})
	return cachedCountry
}

// detectCountryByIP races multiple geolocation services concurrently and returns
// the first successful 2-letter country code.
func detectCountryByIP() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	providers := []func(context.Context) (string, error){
		detectByCloudflare,
		detectByCountryIs,
		detectByIPInfo,
		detectByIPApi,
	}

	type result struct {
		country string
		err     error
	}
	ch := make(chan result, len(providers))

	for _, fn := range providers {
		go func(f func(context.Context) (string, error)) {
			cc, err := f(ctx)
			ch <- result{cc, err}
		}(fn)
	}

	for range providers {
		r := <-ch
		if r.err == nil && len(r.country) == 2 {
			cancel()
			return r.country, nil
		}
	}
	return "", fmt.Errorf("all geolocation services failed")
}

var geoHTTPClient = &http.Client{Timeout: time.Second}

func httpGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return geoHTTPClient.Do(req)
}

// detectByCloudflare uses Cloudflare's /cdn-cgi/trace endpoint.
// Global 300+ PoPs, accessible in China, no API key required.
func detectByCloudflare(ctx context.Context) (string, error) {
	resp, err := httpGet(ctx, "https://1.1.1.1/cdn-cgi/trace")
	if err != nil {
		return "", fmt.Errorf("cloudflare: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cloudflare status: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512))
	if err != nil {
		return "", fmt.Errorf("cloudflare read: %w", err)
	}
	for line := range strings.SplitSeq(string(body), "\n") {
		if strings.HasPrefix(line, "loc=") {
			return strings.TrimSpace(line[4:]), nil
		}
	}
	return "", fmt.Errorf("cloudflare: loc field not found")
}

// detectByCountryIs uses api.country.is. Minimal JSON, Cloudflare CDN, no key.
func detectByCountryIs(ctx context.Context) (string, error) {
	resp, err := httpGet(ctx, "https://api.country.is")
	if err != nil {
		return "", fmt.Errorf("country.is: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("country.is status: %d", resp.StatusCode)
	}
	var result struct {
		Country string `json:"country"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 512)).Decode(&result); err != nil {
		return "", fmt.Errorf("country.is decode: %w", err)
	}
	return result.Country, nil
}

// detectByIPInfo uses ipinfo.io. Well-known service, free 50k/month, no key required.
func detectByIPInfo(ctx context.Context) (string, error) {
	resp, err := httpGet(ctx, "https://ipinfo.io/json")
	if err != nil {
		return "", fmt.Errorf("ipinfo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ipinfo status: %d", resp.StatusCode)
	}
	var result struct {
		Country string `json:"country"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&result); err != nil {
		return "", fmt.Errorf("ipinfo decode: %w", err)
	}
	return result.Country, nil
}

// detectByIPApi uses ip-api.com. Very fast, free unlimited, HTTP only, no key required.
func detectByIPApi(ctx context.Context) (string, error) {
	resp, err := httpGet(ctx, "http://ip-api.com/json/?fields=countryCode")
	if err != nil {
		return "", fmt.Errorf("ip-api: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ip-api status: %d", resp.StatusCode)
	}
	var result struct {
		CountryCode string `json:"countryCode"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 512)).Decode(&result); err != nil {
		return "", fmt.Errorf("ip-api decode: %w", err)
	}
	return result.CountryCode, nil
}
