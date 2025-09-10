package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Mock Kraken server for testing
func mockKrakenServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pair := r.URL.Query().Get("pair")

		response := KrakenResponse{
			Error:  []string{},
			Result: make(map[string]KrakenTickData),
		}

		switch pair {
		case "XXBTZUSD":
			response.Result["XXBTZUSD"] = KrakenTickData{
				C: []string{"45000.00", "0.5"},
			}
		case "XBTCHF":
			response.Result["XBTCHF"] = KrakenTickData{
				C: []string{"41000.00", "0.3"},
			}
		case "XXBTZEUR":
			response.Result["XXBTZEUR"] = KrakenTickData{
				C: []string{"42000.00", "0.4"},
			}
		default:
			response.Error = []string{"Unknown pair"}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}

func TestGetKrakenPair(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"BTC/USD", "XXBTZUSD"},
		{"btc/usd", "XXBTZUSD"},
		{"BTC/CHF", "XBTCHF"},
		{"BTC/EUR", "XXBTZEUR"},
		{"INVALID", ""},
	}

	for _, test := range tests {
		result := getKrakenPair(test.input)
		if result != test.expected {
			t.Errorf("getKrakenPair(%s) = %s; want %s", test.input, result, test.expected)
		}
	}
}

func TestHandleLTP_AllPairs(t *testing.T) {
	service := NewService()

	req := httptest.NewRequest("GET", "/api/v1/ltp", nil)
	rec := httptest.NewRecorder()

	// Mock the Kraken API
	mockServer := mockKrakenServer()
	defer mockServer.Close()

	// Override the Kraken API URL for testing
	service.krakenClient = mockServer.Client()

	// Note: In production code, you'd want to make the base URL configurable
	// For this test, we're using the mock server

	service.handleLTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var response LTPResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response.LTP) == 0 {
		t.Error("Expected at least one LTP entry")
	}
}

func TestHandleLTP_SinglePair(t *testing.T) {
	service := NewService()

	req := httptest.NewRequest("GET", "/api/v1/ltp?pair=BTC/USD", nil)
	rec := httptest.NewRecorder()

	mockServer := mockKrakenServer()
	defer mockServer.Close()

	service.krakenClient = mockServer.Client()

	service.handleLTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var response LTPResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response.LTP) != 1 {
		t.Errorf("Expected 1 LTP entry, got %d", len(response.LTP))
	}

	if response.LTP[0].Pair != "BTC/USD" {
		t.Errorf("Expected pair BTC/USD, got %s", response.LTP[0].Pair)
	}
}

func TestHandleLTP_MultiplePairs(t *testing.T) {
	service := NewService()

	req := httptest.NewRequest("GET", "/api/v1/ltp?pairs=BTC/USD,BTC/EUR", nil)
	rec := httptest.NewRecorder()

	mockServer := mockKrakenServer()
	defer mockServer.Close()

	service.krakenClient = mockServer.Client()

	service.handleLTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var response LTPResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response.LTP) != 2 {
		t.Errorf("Expected 2 LTP entries, got %d", len(response.LTP))
	}
}

func TestHandleLTP_InvalidMethod(t *testing.T) {
	service := NewService()

	req := httptest.NewRequest("POST", "/api/v1/ltp", nil)
	rec := httptest.NewRecorder()

	service.handleLTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", rec.Code)
	}
}

func TestCache(t *testing.T) {
	cache := &Cache{
		data: make(map[string]CacheEntry),
		ttl:  100 * time.Millisecond,
	}

	callCount := 0
	fetcher := func() (float64, error) {
		callCount++
		return 100.0, nil
	}

	// First call should fetch
	val1, err := cache.GetOrFetch("test", fetcher)
	if err != nil || val1 != 100.0 || callCount != 1 {
		t.Errorf("First fetch failed: val=%f, err=%v, calls=%d", val1, err, callCount)
	}

	// Second call should use cache
	val2, err := cache.GetOrFetch("test", fetcher)
	if err != nil || val2 != 100.0 || callCount != 1 {
		t.Errorf("Cache not used: val=%f, err=%v, calls=%d", val2, err, callCount)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Third call should fetch again
	val3, err := cache.GetOrFetch("test", fetcher)
	if err != nil || val3 != 100.0 || callCount != 2 {
		t.Errorf("Cache not expired: val=%f, err=%v, calls=%d", val3, err, callCount)
	}
}

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if rec.Body.String() != "OK" {
		t.Errorf("Expected body 'OK', got '%s'", rec.Body.String())
	}
}
