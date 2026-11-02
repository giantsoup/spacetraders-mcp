package docs

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const maxResultBytes = 128 << 10

func Register(s *server.MCPServer, catalog *Catalog) {
	options := []mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	}
	searchOptions := append([]mcp.ToolOption{}, options...)
	searchOptions = append(searchOptions,
		mcp.WithDescription("Search official SpaceTraders OpenAPI endpoint documentation. All query words must match. Returns operation IDs for get_operation, API version, source URL, hash, and retrieval time. Does not read game state."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Words to find in operation IDs, paths, summaries, descriptions, or tags.")),
		mcp.WithNumber("limit", mcp.Min(1), mcp.Max(20), mcp.MultipleOf(1), mcp.DefaultNumber(10), mcp.Description("Maximum results, 1 to 20. Defaults to 10.")),
	)
	s.AddTool(mcp.NewTool("search_docs", searchOptions...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil || strings.TrimSpace(query) == "" || len(query) > 500 {
			return mcp.NewToolResultError("query must be a nonempty string of at most 500 bytes"), nil
		}
		limit := 10
		if value, exists := request.GetArguments()["limit"]; exists {
			number, ok := value.(float64)
			if !ok || number < 1 || number > 20 || number != float64(int(number)) {
				return mcp.NewToolResultError("limit must be an integer from 1 to 20"), nil
			}
			limit = int(number)
		}
		snapshot, err := catalog.Load(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		results, total := snapshot.Search(query, limit)
		return result(map[string]interface{}{"source": snapshot.Source, "results": results, "total": total})
	})

	operationOptions := append([]mcp.ToolOption{}, options...)
	operationOptions = append(operationOptions,
		mcp.WithDescription("Read an official SpaceTraders OpenAPI operation, including request/response definitions, inherited path parameters, default security, and transitive referenced components. This documents an endpoint; it never executes it. External references are reported without fetching them."),
		mcp.WithString("operation_id", mcp.Required(), mcp.Description("Exact operation_id returned by search_docs.")),
	)
	s.AddTool(mcp.NewTool("get_operation", operationOptions...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := request.RequireString("operation_id")
		if err != nil || strings.TrimSpace(id) == "" || len(id) > 500 {
			return mcp.NewToolResultError("operation_id must be a nonempty string of at most 500 bytes"), nil
		}
		snapshot, err := catalog.Load(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		value, err := snapshot.GetOperation(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return result(value)
	})

	schemaOptions := append([]mcp.ToolOption{}, options...)
	schemaOptions = append(schemaOptions,
		mcp.WithDescription("Read a named component schema from the official SpaceTraders OpenAPI document, with transitive local references and source provenance. Does not read game state."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Exact components.schemas name, such as ShipNav or ShipCargo.")),
	)
	s.AddTool(mcp.NewTool("get_schema", schemaOptions...), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := request.RequireString("name")
		if err != nil || strings.TrimSpace(name) == "" || len(name) > 500 {
			return mcp.NewToolResultError("name must be a nonempty string of at most 500 bytes"), nil
		}
		snapshot, err := catalog.Load(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		value, err := snapshot.GetSchema(name)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return result(value)
	})
}

func result(value interface{}) (*mcp.CallToolResult, error) {
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return mcp.NewToolResultError("encode documentation result: " + err.Error()), nil
	}
	if data.Len() > maxResultBytes {
		return mcp.NewToolResultError("documentation result exceeds 128 KiB; narrow the search or request a smaller schema"), nil
	}
	return mcp.NewToolResultText(data.String()), nil
}
