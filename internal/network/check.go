// Package network checks regional Bitwarden HTTPS reachability.
package network

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Reachable reports a completed HTTPS response, regardless of its HTTP status.
func Reachable(ctx context.Context, endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: time.Second}
	result, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = result.Body.Close()
	return true
}

// Both checks regional endpoints simultaneously within a shared time limit.
func Both(ctx context.Context, us, eu string) [2]bool {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var status [2]bool
	var wg sync.WaitGroup
	for i, endpoint := range []string{us, eu} {
		wg.Add(1)
		go func(index int, address string) {
			defer wg.Done()
			status[index] = Reachable(ctx, address)
		}(i, endpoint)
	}
	wg.Wait()
	return status
}
