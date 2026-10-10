package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInternetProbeRejectsCaptivePortalAndUsesFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/portal":
			w.Write([]byte("Sign in to Wi-Fi"))
		case "/connect":
			w.Write([]byte("Microsoft Connect Test"))
		case "/redirect":
			http.Redirect(w, r, "/connect", http.StatusFound)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for _, path := range []string{"/portal", "/redirect"} {
		if internetAvailable(context.Background(), client, []internetProbe{{server.URL + path, "Microsoft Connect Test"}}) {
			t.Fatal("captive portal/redirect counted as internet")
		}
	}
	if !internetAvailable(context.Background(), client, []internetProbe{{server.URL + "/portal", "Microsoft Connect Test"}, {server.URL + "/connect", "Microsoft Connect Test"}}) {
		t.Fatal("valid fallback did not establish internet reachability")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	if internetAvailable(ctx, client, []internetProbe{{server.URL, ""}}) {
		t.Fatal("expired check accepted connectivity")
	}
}
