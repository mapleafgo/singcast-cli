package translator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DetectCountry determines the user's two-letter country code (ISO 3166-1 alpha-2).
// Priority: explicit argument > IP geolocation > fallback "CN".
func DetectCountry(override string) string {
	if cc := strings.TrimSpace(override); len(cc) == 2 {
		return strings.ToUpper(cc)
	}
	if cc, err := detectCountryByIP(); err == nil && len(cc) == 2 {
		return strings.ToUpper(cc)
	}
	return "CN"
}

// detectCountryByIP queries ipinfo.io for the country of the current public IP.
func detectCountryByIP() (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://ipinfo.io/json")
	if err != nil {
		return "", fmt.Errorf("ip geolocation request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ip geolocation status: %d", resp.StatusCode)
	}
	var result struct {
		Country string `json:"country"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ip geolocation decode: %w", err)
	}
	return result.Country, nil
}
