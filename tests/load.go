package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type ShortenResponse struct {
	ShortCode string `json:"short_code"`
}

func main() {
	server := "http://localhost:8080"

	fmt.Println("=== Phase 1: Create short URLs ===")
	shortCodes := createShortLinks(server, 100)
	fmt.Printf("Created %d short URLs\n\n", len(shortCodes))

	fmt.Println("=== Phase 2: Access short URLs to trigger cache hits ===")
	accessShortLinks(server, shortCodes, 10) // Access each 10 times
	fmt.Println("\n=== Phase 3: Trigger rate limiting ===")
	triggerRateLimit(server)

	fmt.Println("\n=== Done ===")
	fmt.Println("Check Prometheus at http://localhost:9090")
	fmt.Println("Queries:")
	fmt.Println("  - rate(cache_hit_total[1m])  # Cache hit rate")
	fmt.Println("  - rate(cache_miss_total[1m]) # Cache miss rate")
	fmt.Println("  - rate(rate_limit_exceeded_total[1m]) # Rate limit triggers")
}

// Phase 1: Create short URLs
func createShortLinks(server string, count int) []string {
	url := server + "/api/shorten"
	body := bytes.NewBufferString(`{"long_url":"https://example.com/loadtest"}`)

	var mu sync.Mutex
	codes := make([]string, 0, count)
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req, _ := http.NewRequest("POST", url, body)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == 200 {
				var result ShortenResponse
				json.NewDecoder(resp.Body).Decode(&result)
				mu.Lock()
				codes = append(codes, result.ShortCode)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	return codes
}

// Phase 2: Access short URLs multiple times to warm up cache
func accessShortLinks(server string, codes []string, times int) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirect
		},
	}

	var wg sync.WaitGroup
	for _, code := range codes {
		for i := 0; i < times; i++ {
			wg.Add(1)
			go func(c string) {
				defer wg.Done()
				resp, _ := client.Get(server + "/" + c)
				if resp != nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			}(code)
		}
	}
	wg.Wait()
	fmt.Printf("Accessed %d short codes × %d times = %d requests\n", len(codes), times, len(codes)*times)
}

// Phase 3: Trigger rate limiting
func triggerRateLimit(server string) {
	url := server + "/api/shorten"
	body := bytes.NewBufferString(`{"long_url":"https://ratelimit.test"}`)

	fmt.Println("Sending rapid requests to trigger rate limit...")

	var mu sync.Mutex
	rateLimited := 0
	var wg sync.WaitGroup

	// Send 200 requests rapidly from 10 goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			client := &http.Client{Timeout: 2 * time.Second}
			for j := 0; j < 20; j++ {
				req, _ := http.NewRequest("POST", url, body)
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				if err != nil {
					continue
				}
				if resp.StatusCode == http.StatusTooManyRequests {
					mu.Lock()
					rateLimited++
					mu.Unlock()
				}
				resp.Body.Close()
				time.Sleep(time.Millisecond * 20) // Keep rate above 20 QPS
			}
		}(i)
	}
	wg.Wait()

	fmt.Printf("Rate limited requests: %d\n", rateLimited)
}
