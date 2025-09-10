package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Response structures
type LTPResponse struct {
	LTP []PairLTP `json:"ltp"`
}

type PairLTP struct {
	Pair   string  `json:"pair"`
	Amount float64 `json:"amount"`
}

// Kraken API response structures
type KrakenResponse struct {
	Error  []string                  `json:"error"`
	Result map[string]KrakenTickData `json:"result"`
}

type KrakenTickData struct {
	C []string `json:"c"` // Close price [price, lot volume]
}

// Service structure
type Service struct {
	krakenClient *http.Client
	cache        *Cache
}

// Cache structure for rate limiting protection
type Cache struct {
	data map[string]CacheEntry
	ttl  time.Duration
}

type CacheEntry struct {
	value     float64
	timestamp time.Time
}

// NewService creates a new service instance
func NewService() *Service {
	return &Service{
		krakenClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: &Cache{
			data: make(map[string]CacheEntry),
			ttl:  30 * time.Second, // Cache for 30 seconds
		},
	}
}

// Get cached value or fetch new one
func (c *Cache) GetOrFetch(pair string, fetcher func() (float64, error)) (float64, error) {
	if entry, exists := c.data[pair]; exists {
		if time.Since(entry.timestamp) < c.ttl {
			return entry.value, nil
		}
	}

	value, err := fetcher()
	if err != nil {
		return 0, err
	}

	c.data[pair] = CacheEntry{
		value:     value,
		timestamp: time.Now(),
	}

	return value, nil
}

// Map internal pair names to Kraken pair names
func getKrakenPair(pair string) string {
	switch strings.ToUpper(pair) {
	case "BTC/USD":
		return "XXBTZUSD"
	case "BTC/CHF":
		return "XBTCHF"
	case "BTC/EUR":
		return "XXBTZEUR"
	default:
		return ""
	}
}

// Fetch LTP from Kraken API
func (s *Service) fetchLTPFromKraken(pair string) (float64, error) {
	krakenPair := getKrakenPair(pair)
	if krakenPair == "" {
		return 0, fmt.Errorf("unsupported pair: %s", pair)
	}

	url := fmt.Sprintf("https://api.kraken.com/0/public/Ticker?pair=%s", krakenPair)

	resp, err := s.krakenClient.Get(url)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch from Kraken: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	var krakenResp KrakenResponse
	if err := json.Unmarshal(body, &krakenResp); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(krakenResp.Error) > 0 {
		return 0, fmt.Errorf("Kraken API error: %v", krakenResp.Error)
	}

	tickData, exists := krakenResp.Result[krakenPair]
	if !exists {
		return 0, fmt.Errorf("no data for pair %s", pair)
	}

	if len(tickData.C) == 0 {
		return 0, fmt.Errorf("no close price for pair %s", pair)
	}

	price, err := strconv.ParseFloat(tickData.C[0], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse price: %w", err)
	}

	return price, nil
}

// Get LTP for a single pair or multiple pairs
func (s *Service) getLTP(pairs []string) ([]PairLTP, error) {
	result := make([]PairLTP, 0, len(pairs))

	for _, pair := range pairs {
		pair = strings.ToUpper(strings.TrimSpace(pair))

		amount, err := s.cache.GetOrFetch(pair, func() (float64, error) {
			return s.fetchLTPFromKraken(pair)
		})

		if err != nil {
			log.Printf("Error fetching LTP for %s: %v", pair, err)
			continue
		}

		result = append(result, PairLTP{
			Pair:   pair,
			Amount: amount,
		})
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("failed to fetch any LTP data")
	}

	return result, nil
}

// HTTP handler for /api/v1/ltp
func (s *Service) handleLTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	pairParam := r.URL.Query().Get("pair")
	pairsParam := r.URL.Query().Get("pairs")

	var pairs []string

	if pairParam != "" {
		// Single pair
		pairs = []string{pairParam}
	} else if pairsParam != "" {
		// Multiple pairs (comma-separated)
		pairs = strings.Split(pairsParam, ",")
	} else {
		// Default to all supported pairs
		pairs = []string{"BTC/USD", "BTC/CHF", "BTC/EUR"}
	}

	// Get LTP data
	ltpData, err := s.getLTP(pairs)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching LTP: %v", err), http.StatusInternalServerError)
		return
	}

	// Create response
	response := LTPResponse{
		LTP: ltpData,
	}

	// Set headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Encode and send response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

// Health check endpoint
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func main() {
	service := NewService()

	// Setup routes
	http.HandleFunc("/api/v1/ltp", service.handleLTP)
	http.HandleFunc("/health", handleHealth)

	// Start server
	port := "8080"
	log.Printf("Starting server on port %s", port)
	log.Printf("Endpoints:")
	log.Printf("  GET /api/v1/ltp - Get all pairs")
	log.Printf("  GET /api/v1/ltp?pair=BTC/USD - Get single pair")
	log.Printf("  GET /api/v1/ltp?pairs=BTC/USD,BTC/EUR - Get multiple pairs")
	log.Printf("  GET /health - Health check")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
