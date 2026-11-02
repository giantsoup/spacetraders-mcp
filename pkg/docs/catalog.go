// Package docs reads the public OpenAPI document without game credentials.
package docs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	SourceURL = "https://api.spacetraders.io/v2/documentation/json"
	cacheTTL  = 15 * time.Minute
	maxBytes  = 8 << 20
)

type Source struct {
	URL         string    `json:"url"`
	APIVersion  string    `json:"api_version"`
	OpenAPI     string    `json:"openapi"`
	SHA256      string    `json:"sha256"`
	RetrievedAt time.Time `json:"retrieved_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Operation struct {
	ID          string   `json:"operation_id"`
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Summary     string   `json:"summary"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	raw         json.RawMessage
	parameters  json.RawMessage
}

type document struct {
	OpenAPI string `json:"openapi"`
	Info    struct {
		Version string `json:"version"`
	} `json:"info"`
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components map[string]map[string]json.RawMessage `json:"components"`
	Security   json.RawMessage                       `json:"security"`
	Servers    json.RawMessage                       `json:"servers"`
}

type Snapshot struct {
	Source     Source
	document   document
	operations []Operation
}

type Catalog struct {
	mu       sync.Mutex
	client   *http.Client
	snapshot *Snapshot
}

func NewCatalog() *Catalog {
	return &Catalog{client: &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return fmt.Errorf("documentation redirects are not supported")
		},
	}}
}

// Load fetches only the fixed documentation URL and reuses a fresh in-memory snapshot.
// A failed refresh returns an error instead of silently serving an expired document.
func (c *Catalog) Load(ctx context.Context) (*Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snapshot != nil && time.Now().Before(c.snapshot.Source.ExpiresAt) {
		return c.snapshot, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, SourceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch official documentation: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("official documentation returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read official documentation: %w", err)
	}
	if len(body) > maxBytes {
		return nil, fmt.Errorf("official documentation exceeds %d bytes", maxBytes)
	}
	snapshot, err := parse(body, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	c.snapshot = snapshot
	return snapshot, nil
}

func parse(body []byte, retrievedAt time.Time) (*Snapshot, error) {
	var doc document
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse official documentation: %w", err)
	}
	if !strings.HasPrefix(doc.OpenAPI, "3.") || doc.Info.Version == "" || len(doc.Paths) == 0 {
		return nil, fmt.Errorf("official documentation must contain OpenAPI 3, an API version, and paths")
	}
	digest := sha256.Sum256(body)
	s := &Snapshot{document: doc, Source: Source{
		URL: SourceURL, APIVersion: doc.Info.Version, OpenAPI: doc.OpenAPI,
		SHA256: hex.EncodeToString(digest[:]), RetrievedAt: retrievedAt,
		ExpiresAt: retrievedAt.Add(cacheTTL),
	}}
	ids := make(map[string]bool)
	for path, item := range doc.Paths {
		for _, method := range []string{"get", "post", "put", "patch", "delete", "head", "options", "trace"} {
			raw, ok := item[method]
			if !ok {
				continue
			}
			var fields struct {
				ID          string   `json:"operationId"`
				Summary     string   `json:"summary"`
				Description string   `json:"description"`
				Tags        []string `json:"tags"`
			}
			if err := json.Unmarshal(raw, &fields); err != nil || string(raw) == "null" {
				return nil, fmt.Errorf("invalid documentation operation %s %s", method, path)
			}
			id := fields.ID
			if id == "" {
				id = strings.ToUpper(method) + " " + path
			}
			if ids[id] {
				return nil, fmt.Errorf("duplicate documentation operation ID %q", id)
			}
			ids[id] = true
			s.operations = append(s.operations, Operation{
				ID: id, Method: strings.ToUpper(method), Path: path, Summary: fields.Summary,
				Description: fields.Description, Tags: fields.Tags, raw: raw, parameters: item["parameters"],
			})
		}
	}
	if len(s.operations) == 0 {
		return nil, fmt.Errorf("official documentation contains no operations")
	}
	sort.Slice(s.operations, func(i, j int) bool { return s.operations[i].ID < s.operations[j].ID })
	return s, nil
}

func (s *Snapshot) Search(query string, limit int) ([]Operation, int) {
	terms := strings.Fields(strings.ToLower(query))
	results := make([]Operation, 0)
	total := 0
	for _, op := range s.operations {
		text := strings.ToLower(strings.Join([]string{op.ID, op.Method, op.Path, op.Summary, op.Description, strings.Join(op.Tags, " ")}, " "))
		matches := true
		for _, term := range terms {
			if !strings.Contains(text, term) {
				matches = false
				break
			}
		}
		if matches {
			total++
			if len(results) < limit {
				results = append(results, op)
			}
		}
	}
	return results, total
}

func (s *Snapshot) GetOperation(id string) (map[string]interface{}, error) {
	for _, op := range s.operations {
		if op.ID != id {
			continue
		}
		components, unresolved := s.references(op.raw, op.parameters)
		components["securitySchemes"] = s.document.Components["securitySchemes"]
		return map[string]interface{}{
			"source": s.Source, "method": op.Method, "path": op.Path, "operation": op.raw,
			"path_parameters": op.parameters, "default_security": s.document.Security,
			"servers": s.document.Servers, "components": components, "unresolved_refs": unresolved,
		}, nil
	}
	return nil, fmt.Errorf("unknown operation %q; use search_docs to find an operation_id", id)
}

func (s *Snapshot) GetSchema(name string) (map[string]interface{}, error) {
	raw, ok := s.document.Components["schemas"][name]
	if !ok {
		return nil, fmt.Errorf("unknown schema %q", name)
	}
	components, unresolved := s.references(raw)
	return map[string]interface{}{
		"source": s.Source, "name": name, "schema": raw,
		"components": components, "unresolved_refs": unresolved,
	}, nil
}

// Preserve $refs and include their transitive local definitions, including cycles.
// External references are reported, never fetched.
func (s *Snapshot) references(values ...json.RawMessage) (map[string]map[string]json.RawMessage, []string) {
	components := make(map[string]map[string]json.RawMessage)
	seen := make(map[string]bool)
	unresolved := make([]string, 0)
	var walk func(interface{})
	var scan func(json.RawMessage)
	scan = func(raw json.RawMessage) {
		var value interface{}
		if json.Unmarshal(raw, &value) == nil {
			walk(value)
		}
	}
	walk = func(value interface{}) {
		switch value := value.(type) {
		case map[string]interface{}:
			if ref, ok := value["$ref"].(string); ok && !seen[ref] {
				seen[ref] = true
				parts := strings.Split(strings.TrimPrefix(ref, "#/components/"), "/")
				if !strings.HasPrefix(ref, "#/components/") || len(parts) != 2 {
					unresolved = append(unresolved, ref)
				} else {
					decode := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~") }
					group, name := decode(parts[0]), decode(parts[1])
					if raw, ok := s.document.Components[group][name]; ok {
						if components[group] == nil {
							components[group] = make(map[string]json.RawMessage)
						}
						components[group][name] = raw
						scan(raw)
					} else {
						unresolved = append(unresolved, ref)
					}
				}
			}
			for _, child := range value {
				walk(child)
			}
		case []interface{}:
			for _, child := range value {
				walk(child)
			}
		}
	}
	for _, raw := range values {
		scan(raw)
	}
	sort.Strings(unresolved)
	return components, unresolved
}
