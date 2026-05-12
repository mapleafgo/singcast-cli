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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	providers := []func(context.Context) (string, error){
		detectByIPSb,
		detectByIPWhoIs,
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

var geoHTTPClient = &http.Client{}

func httpGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return geoHTTPClient.Do(req)
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

// detectByIPSb uses api.ip.sb. Fast, China-friendly, no key required.
func detectByIPSb(ctx context.Context) (string, error) {
	resp, err := httpGet(ctx, "https://api.ip.sb/geoip")
	if err != nil {
		return "", fmt.Errorf("ip.sb: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ip.sb status: %d", resp.StatusCode)
	}
	var result struct {
		CountryCode string `json:"country_code"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 512)).Decode(&result); err != nil {
		return "", fmt.Errorf("ip.sb decode: %w", err)
	}
	return result.CountryCode, nil
}

// detectByIPWhoIs uses ipwho.is. Fast, no key required.
func detectByIPWhoIs(ctx context.Context) (string, error) {
	resp, err := httpGet(ctx, "https://ipwho.is/")
	if err != nil {
		return "", fmt.Errorf("ipwho.is: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ipwho.is status: %d", resp.StatusCode)
	}
	var result struct {
		Success     bool   `json:"success"`
		CountryCode string `json:"country_code"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&result); err != nil {
		return "", fmt.Errorf("ipwho.is decode: %w", err)
	}
	if !result.Success {
		return "", fmt.Errorf("ipwho.is: request unsuccessful")
	}
	return result.CountryCode, nil
}
