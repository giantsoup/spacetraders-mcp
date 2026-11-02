package docs

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestFailedLoadsDoNotStampede(t *testing.T) {
	catalog, count := testCatalog(t, fixture, http.StatusServiceUnavailable)
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := catalog.Load(context.Background()); err == nil {
				t.Error("failed source returned success")
			}
		}()
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatalf("concurrent failed lookups made %d fetches, want one shared failure", count.Load())
	}
}

func TestToolSchemaAdvertisesLimits(t *testing.T) {
	s := server.NewMCPServer("test", "1")
	Register(s, NewCatalog())
	property := s.GetTool("search_docs").Tool.InputSchema.Properties["limit"].(map[string]interface{})
	for name, want := range map[string]float64{"minimum": 1, "maximum": 20, "multipleOf": 1, "default": 10} {
		if property[name] != want {
			t.Errorf("limit %s = %v, want %v", name, property[name], want)
		}
	}
}

func TestResultPreservesJSON(t *testing.T) {
	value := map[string]string{"description": "<example>& documentation"}
	response, err := result(value)
	if err != nil || response.IsError {
		t.Fatalf("small result failed: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(response.Content[0].(mcp.TextContent).Text), &decoded); err != nil || decoded["description"] != value["description"] {
		t.Fatalf("documentation content changed: %v", err)
	}
}

func TestHTTPSRedirectAndDecodedSizeLimit(t *testing.T) {
	for _, mode := range []string{"redirect", "gzip"} {
		t.Run(mode, func(t *testing.T) {
			var requests atomic.Int32
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/v2/documentation/json" || r.Header.Get("Authorization") != "" {
					t.Error("request crossed documentation boundary")
				}
				if mode == "redirect" {
					http.Redirect(w, r, "/not-documentation", http.StatusFound)
					return
				}
				w.Header().Set("Content-Encoding", "gzip")
				compressed := gzip.NewWriter(w)
				_, _ = compressed.Write([]byte(strings.Repeat(" ", maxBytes+1)))
				_ = compressed.Close()
			}))
			defer s.Close()
			catalog := NewCatalog()
			transport := s.Client().Transport.(*http.Transport).Clone()
			transport.TLSClientConfig = &tls.Config{RootCAs: transport.TLSClientConfig.RootCAs, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}
			// Route only this fixture's fixed HTTPS request to the local TLS server.
			transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, s.Listener.Addr().String())
			}
			catalog.client.Transport = transport
			_, err := catalog.Load(context.Background())
			want := "redirects are not supported"
			if mode == "gzip" {
				want = "exceeds"
			}
			if err == nil || !strings.Contains(err.Error(), want) || requests.Load() != 1 {
				t.Fatalf("%s boundary failed: %v, requests %d", mode, err, requests.Load())
			}
			transport.CloseIdleConnections()
		})
	}
}

func TestDocumentationResultLimit(t *testing.T) {
	value := map[string]string{"description": strings.Repeat("<", 1<<20)}
	response, err := result(value)
	if err != nil || !response.IsError {
		t.Fatalf("oversized documentation result was not rejected: %v", err)
	}
}

func TestCancellationDuringFetch(t *testing.T) {
	catalog := NewCatalog()
	started := make(chan struct{})
	catalog.client.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := catalog.Load(ctx); done <- err }()
	<-started
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled fetch returned success")
	}
	// A cancelled caller must not poison the next caller's fetch.
	catalog.client.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fixture))}, nil
	})
	if _, err := catalog.Load(context.Background()); err != nil {
		t.Fatal("caller cancellation poisoned a later fetch:", err)
	}
}

type observedContext struct {
	context.Context
	observed chan struct{}
}

func (c observedContext) Done() <-chan struct{} {
	select {
	case c.observed <- struct{}{}:
	default:
	}
	return c.Context.Done()
}

func TestQueuedCallerCancellation(t *testing.T) {
	catalog := NewCatalog()
	started, release := make(chan struct{}), make(chan struct{})
	catalog.client.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-release
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fixture))}, nil
	})
	first := make(chan error, 1)
	go func() { _, err := catalog.Load(context.Background()); first <- err }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	observed := make(chan struct{}, 1)
	waiter := make(chan error, 1)
	go func() { _, err := catalog.Load(observedContext{ctx, observed}); waiter <- err }()
	select {
	case <-observed:
	case <-time.After(time.Second):
		t.Error("queued caller never observed its cancellation channel")
	}
	cancel()
	returned := false
	select {
	case err := <-waiter:
		returned = true
		if !errors.Is(err, context.Canceled) {
			t.Errorf("cancelled waiter returned %v", err)
		}
	case <-time.After(time.Second):
		t.Error("cancelled caller remained blocked behind the network fetch")
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if !returned {
		if err := <-waiter; !errors.Is(err, context.Canceled) {
			t.Errorf("cancelled caller eventually returned %v", err)
		}
	}
}
