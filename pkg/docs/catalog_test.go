package docs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const fixture = `{
  "openapi":"3.0.1", "info":{"version":"2.3.0"},
  "servers":[{"url":"https://api.spacetraders.io/v2"}],
  "security":[{"AgentToken":[]}],
  "paths":{
    "/my/ships/{shipSymbol}/navigate":{
      "parameters":[{"$ref":"#/components/parameters/ShipSymbol"}],
      "post":{"operationId":"navigate-ship","summary":"Navigate ship","description":"Travel to a waypoint", "tags":["Fleet"],
        "requestBody":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/NavigateRequest"}}}},
        "responses":{"200":{"$ref":"#/components/responses/Navigation"}}}
    },
    "/my/ships":{"get":{"operationId":"get-my-ships","summary":"List fleet"}},
    "/status":{"get":{"summary":"Status without an operation ID"}}
  },
  "components":{
    "schemas":{
      "NavigateRequest":{"type":"object","required":["waypointSymbol"],"properties":{"waypointSymbol":{"type":"string"}}},
      "ShipNav":{"type":"object","properties":{"route":{"$ref":"#/components/schemas/Route"}}},
      "Route":{"type":"object","properties":{"nav":{"$ref":"#/components/schemas/ShipNav"}}},
      "External":{"$ref":"https://example.invalid/schema.json"},
      "Missing":{"$ref":"#/components/schemas/DoesNotExist"},
      "A/B":{"type":"string"},
      "Escaped":{"$ref":"#/components/schemas/A~1B"}
    },
    "parameters":{"ShipSymbol":{"name":"shipSymbol","in":"path","required":true,"schema":{"type":"string"}}},
    "responses":{"Navigation":{"description":"Navigation result","content":{"application/json":{"schema":{"$ref":"#/components/schemas/ShipNav"}}}}},
    "securitySchemes":{"AgentToken":{"type":"http","scheme":"bearer"}}
  }
}`

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func testCatalog(t *testing.T, body string, status int) (*Catalog, *atomic.Int32) {
	t.Helper()
	count := &atomic.Int32{}
	catalog := NewCatalog()
	catalog.client.Transport = transportFunc(func(req *http.Request) (*http.Response, error) {
		count.Add(1)
		if req.URL.String() != SourceURL || req.Method != http.MethodGet || req.Header.Get("Authorization") != "" {
			t.Errorf("unexpected documentation request: %s %s, authorization present: %t", req.Method, req.URL, req.Header.Get("Authorization") != "")
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	return catalog, count
}

func TestSearchAndOperationReferences(t *testing.T) {
	s, err := parse([]byte(fixture), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	results, total := s.Search("FLEET waypoint", 1)
	if total != 1 || len(results) != 1 || results[0].ID != "navigate-ship" {
		t.Fatalf("unexpected search: %+v, total %d", results, total)
	}
	all, total := s.Search("", 1)
	if total != 3 || len(all) != 1 {
		t.Fatalf("limit lost total: %d/%d", len(all), total)
	}
	value, err := s.GetOperation("navigate-ship")
	if err != nil {
		t.Fatal(err)
	}
	components := value["components"].(map[string]map[string]json.RawMessage)
	for group, names := range map[string][]string{
		"schemas":    {"NavigateRequest", "ShipNav", "Route"},
		"parameters": {"ShipSymbol"}, "responses": {"Navigation"}, "securitySchemes": {"AgentToken"},
	} {
		for _, name := range names {
			if len(components[group][name]) == 0 {
				t.Errorf("missing transitive definition %s/%s", group, name)
			}
		}
	}
	if len(value["unresolved_refs"].([]string)) != 0 || len(value["path_parameters"].(json.RawMessage)) == 0 {
		t.Fatal("lost path parameters or failed to resolve local references")
	}
	if _, err := s.GetOperation("GET /status"); err != nil {
		t.Fatal("operation without ID must remain discoverable:", err)
	}
	if _, err := s.GetOperation("not-real"); err == nil {
		t.Fatal("unknown operation succeeded")
	}
	if _, err := s.GetSchema("not-real"); err == nil {
		t.Fatal("unknown schema succeeded")
	}
	for _, name := range []string{"External", "Missing", "Escaped"} {
		value, err := s.GetSchema(name)
		if err != nil {
			t.Fatal(err)
		}
		unresolved := value["unresolved_refs"].([]string)
		if name == "Escaped" && len(unresolved) != 0 || name != "Escaped" && len(unresolved) != 1 {
			t.Errorf("unexpected reference resolution for %s: %v", name, unresolved)
		}
	}
}

func TestCacheAndProvenance(t *testing.T) {
	catalog, count := testCatalog(t, fixture, http.StatusOK)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := catalog.Load(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			if s.Source.APIVersion != "2.3.0" || len(s.Source.SHA256) != 64 || s.Source.URL != SourceURL {
				t.Errorf("missing provenance: %+v", s.Source)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatalf("concurrent requests fetched %d documents", count.Load())
	}
	catalog.snapshot.Source.ExpiresAt = time.Now().Add(-time.Second)
	if _, err := catalog.Load(context.Background()); err != nil || count.Load() != 2 {
		t.Fatalf("expired snapshot was not refreshed: %v, requests %d", err, count.Load())
	}
}

func TestFetchErrors(t *testing.T) {
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"http error", fixture, 503}, {"invalid JSON", "<html>error</html>", 200},
		{"invalid spec", `{"paths":{}}`, 200}, {"too large", strings.Repeat(" ", maxBytes+1), 200},
		{"null operation", `{"openapi":"3.0.1","info":{"version":"2"},"paths":{"/":{"get":null}}}`, 200},
		{"no operations", `{"openapi":"3.0.1","info":{"version":"2"},"paths":{"/":{}}}`, 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			catalog, _ := testCatalog(t, test.body, test.status)
			if _, err := catalog.Load(context.Background()); err == nil {
				t.Fatal("invalid response succeeded")
			}
		})
	}
	catalog, _ := testCatalog(t, fixture, 200)
	if err := catalog.client.CheckRedirect(&http.Request{}, nil); err == nil {
		t.Fatal("documentation redirect allowed")
	}
}

func TestToolValidationAndRetrieval(t *testing.T) {
	catalog, count := testCatalog(t, fixture, 200)
	s := server.NewMCPServer("test", "1")
	Register(s, catalog)
	for _, test := range []struct {
		name string
		args map[string]interface{}
	}{
		{"search_docs", map[string]interface{}{"query": " "}},
		{"search_docs", map[string]interface{}{"query": 42}},
		{"search_docs", map[string]interface{}{"query": "ship", "limit": 0.0}},
		{"search_docs", map[string]interface{}{"query": "ship", "limit": 1.5}},
		{"search_docs", map[string]interface{}{"query": "ship", "limit": "2"}},
		{"get_operation", map[string]interface{}{}},
		{"get_schema", map[string]interface{}{"name": ""}},
	} {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = test.args
		result, err := s.GetTool(test.name).Handler(context.Background(), req)
		if err != nil || !result.IsError {
			t.Errorf("invalid %s arguments succeeded: %v", test.name, err)
		}
	}
	if count.Load() != 0 {
		t.Fatal("invalid inputs fetched documentation")
	}
	for name, args := range map[string]map[string]interface{}{
		"search_docs":   {"query": "navigate", "limit": 2.0},
		"get_operation": {"operation_id": "navigate-ship"}, "get_schema": {"name": "ShipNav"},
	} {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = args
		result, err := s.GetTool(name).Handler(context.Background(), req)
		if err != nil || result.IsError {
			t.Fatalf("%s failed: %v, %+v", name, err, result)
		}
		var value map[string]interface{}
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &value); err != nil || value["source"] == nil {
			t.Fatalf("%s result lost JSON or provenance: %v", name, err)
		}
	}
}
