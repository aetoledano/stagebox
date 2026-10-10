package network

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

type internetProbe struct{ url, expected string }

// Check actual reachability, including Ethernet, rather than Wi-Fi association.
func InternetAvailable(ctx context.Context) bool {
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return internetAvailable(ctx, client, []internetProbe{
		{"http://www.msftconnecttest.com/connecttest.txt", "Microsoft Connect Test"},
		{"https://store.steampowered.com/", ""},
	})
}

func internetAvailable(ctx context.Context, client *http.Client, probes []internetProbe) bool {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	results := make(chan bool, len(probes))
	for _, probe := range probes {
		go func(probe internetProbe) {
			method := http.MethodHead
			if probe.expected != "" {
				method = http.MethodGet
			}
			request, err := http.NewRequestWithContext(ctx, method, probe.url, nil)
			if err != nil {
				results <- false
				return
			}
			response, err := client.Do(request)
			if err != nil {
				results <- false
				return
			}
			defer response.Body.Close()
			ok := response.StatusCode >= 200 && response.StatusCode < 300
			if ok && probe.expected != "" {
				body, err := io.ReadAll(io.LimitReader(response.Body, 1024))
				ok = err == nil && strings.TrimSpace(string(body)) == probe.expected
			}
			results <- ok
		}(probe)
	}
	for range probes {
		select {
		case ok := <-results:
			if ok {
				return true
			}
		case <-ctx.Done():
			return false
		}
	}
	return false
}
