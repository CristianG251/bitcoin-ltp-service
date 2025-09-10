//go:build integration
// +build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

const baseURL = "http://localhost:8080"

// TestIntegrationLTPAllPairs tests the real API with all pairs
func TestIntegrationLTPAllPairs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("%s/api/v1/ltp", baseURL))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var response LTPResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check we got all three pairs
	if len(response.LTP) != 3 {
		t.Errorf("Expected 3 pairs, got %d", len(response.LTP))
	}

	// Verify pairs and that amounts are positive
	expectedPairs := map[string]bool{
		"BTC/USD": false,
		"BTC/CHF": false,
		"BTC/EUR": false,
	}

	for _, ltp := range response.LTP {
		if _, exists := expectedPairs[ltp.Pair]; exists {
			expectedPairs[ltp.Pair] = true
			if ltp.Amount <= 0 {
				t.Errorf("Invalid amount for %s: %f", ltp.Pair, ltp.Amount)
			}
		} else {
			t.Errorf("Unexpected pair: %s", ltp.Pair)
		}
	}

	// Check all expected pairs were found
	for pair, found := range expectedPairs {
		if !found {
			t.Errorf("Missing pair: %s", pair)
		}
	}
}

// TestIntegrationLTPSinglePair tests fetching a single pair
func TestIntegrationLTPSinglePair(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("%s/api/v1/ltp?pair=BTC/USD", baseURL))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var response LTPResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response.LTP) != 1 {
		t.Errorf("Expected 1 pair, got %d", len(response.LTP))
	}

	if response.LTP[0].Pair != "BTC/USD" {
		t.Errorf("Expected BTC/USD, got %s", response.LTP[0].Pair)
	}

	if response.LTP[0].Amount <= 0 {
		t.Errorf("Invalid amount: %f", response.LTP[0].Amount)
	}
}

// TestIntegrationLTPMultiplePairs tests fetching multiple pairs
func TestIntegrationLTPMultiplePairs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("%s/api/v1/ltp?pairs=BTC/USD,BTC/EUR", baseURL))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var response LTPResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response.LTP) != 2 {
		t.Errorf("Expected 2 pairs, got %d", len(response.LTP))
	}

	pairs := make(map[string]float64)
	for _, ltp := range response.LTP {
		pairs[ltp.Pair] = ltp.Amount
	}

	if _, exists := pairs["BTC/USD"]; !exists {
		t.Error("Missing BTC/USD pair")
	}

	if _, exists := pairs["BTC/EUR"]; !exists {
		t.Error("Missing BTC/EUR pair")
	}
}

// TestIntegrationCaching tests that caching works correctly
func TestIntegrationCaching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	// First request
	start1 := time.Now()
	resp1, err := client.Get(fmt.Sprintf("%s/api/v1/ltp?pair=BTC/USD", baseURL))
	if err != nil {
		t.Fatalf("Failed to make first request: %v", err)
	}
	duration1 := time.Since(start1)

	var response1 LTPResponse
	if err := json.NewDecoder(resp1.Body).Decode(&response1); err != nil {
		t.Fatalf("Failed to decode first response: %v", err)
	}
	resp1.Body.Close()

	// Second request (should be cached and faster)
	start2 := time.Now()
	resp2, err := client.Get(fmt.Sprintf("%s/api/v1/ltp?pair=BTC/USD", baseURL))
	if err != nil {
		t.Fatalf("Failed to make second request: %v", err)
	}
	duration2 := time.Since(start2)

	var response2 LTPResponse
	if err := json.NewDecoder(resp2.Body).Decode(&response2); err != nil {
		t.Fatalf("Failed to decode second response: %v", err)
	}
	resp2.Body.Close()

	// Values should be the same (cached)
	if response1.LTP[0].Amount != response2.LTP[0].Amount {
		t.Errorf("Expected cached value, got different values: %f vs %f",
			response1.LTP[0].Amount, response2.LTP[0].Amount)
	}

	// Second request should be significantly faster (cached)
	if duration2 > duration1/2 {
		t.Logf("Warning: Cache might not be working efficiently. First: %v, Second: %v",
			duration1, duration2)
	}

	t.Logf("Cache test - First request: %v, Second request: %v", duration1, duration2)
}

// TestIntegrationHealth tests the health endpoint
func TestIntegrationHealth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("%s/health", baseURL))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

// TestIntegrationInvalidPair tests handling of invalid pairs
func TestIntegrationInvalidPair(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("%s/api/v1/ltp?pair=INVALID/PAIR", baseURL))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Should return 500 or empty result
	if resp.StatusCode != http.StatusInternalServerError {
		var response LTPResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}
		// Should have no results for invalid pair
		if len(response.LTP) != 0 {
			t.Errorf("Expected no results for invalid pair, got %d", len(response.LTP))
		}
	}
}

// TestIntegrationRealKrakenAPI tests actual connection to Kraken API
func TestIntegrationRealKrakenAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test direct Kraken API connection
	service := NewService()

	// Test BTC/USD
	amount, err := service.fetchLTPFromKraken("BTC/USD")
	if err != nil {
		t.Errorf("Failed to fetch BTC/USD from Kraken: %v", err)
	}

	if amount <= 0 {
		t.Errorf("Invalid BTC/USD amount: %f", amount)
	}

	// Price sanity check (Bitcoin should be between $1,000 and $1,000,000)
	if amount < 1000 || amount > 1000000 {
		t.Errorf("BTC/USD price seems unrealistic: %f", amount)
	}

	t.Logf("Current BTC/USD price: $%.2f", amount)
}
