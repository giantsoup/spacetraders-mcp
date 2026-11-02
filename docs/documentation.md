# Documentation tools

Run the server with `--docs-only` to look up SpaceTraders API documentation without a game token:

```sh
go build -o spacetraders-mcp .
./spacetraders-mcp --docs-only
```

This is a stdio MCP server. Your client starts it and communicates over stdin/stdout; it does not listen on a network port. In this mode the server skips game configuration, including `.env`, secret files, and secret helpers, and registers only the three documentation tools. It registers no gameplay resources or gameplay prompts. Running without the flag requires an agent credential from the [ordered credential sources](credentials.md) and also adds documentation tools.

Build with a current patched Go release. The `go.mod` language minimum is a compatibility floor, not a recommendation to ship that old compiler's standard library. CI uses the maintained Go 1.26 patch line. [Official Go releases](https://go.dev/dl/) and [Go vulnerability checking](https://go.dev/doc/security/vuln/) describe the available releases and security checks.

## Available tools

- `search_docs` searches operation IDs, HTTP methods, paths, summaries, descriptions, and tags. Supply a nonempty `query`; every word must match. Results are ordered by operation ID. `limit` defaults to 10 and accepts integers from 1 to 20. `total` reports all matches.
- `get_operation` accepts an exact `operation_id` from search results. It returns the original operation, path-level parameters, default security, server URLs, and transitive referenced components. Reading a POST operation's documentation does not execute that operation.
- `get_schema` accepts an exact component schema `name`, such as `ShipNav`. It returns the definition and transitive referenced components. References remain intact and cycles terminate. Direct component pointers such as `#/components/schemas/ShipNav` are supported; missing, external, and unsupported local pointers appear in `unresolved_refs`. The server never follows external references.

For example, search for `navigate`, read the returned `operation_id`, and inspect `ShipNav`. These tools search the OpenAPI reference, not the prose guides on the documentation website.

## Sources and freshness

The tools make an unauthenticated GET to the fixed [official API-hosted OpenAPI JSON](https://api.spacetraders.io/v2/documentation/json). They accept no arbitrary URL, do not forward game credentials, and reject HTTP redirects. The response has a 20-second timeout and an 8 MiB decoded size limit. Tool-result JSON is limited to 128 KiB; larger results return an error suggesting a narrower query or smaller schema.

Each successful result includes the source URL, API version, OpenAPI version, SHA-256 of the downloaded bytes, UTC retrieval time, and cache expiry. The server fetches on the first tool call and keeps the snapshot in memory for 15 minutes, using a monotonic clock for expiration. Startup and listing tools require no network access. Concurrent callers share an in-flight fetch and its result. Upstream failures are cached for one second to avoid queued retries, then the next lookup can retry. Caller cancellations are not cached. Refresh failures return an error instead of silently serving expired data. Restarting the client connection also clears this cache. There is no offline fallback.

The catalog honors cancellation of its Go context while waiting or fetching. The current MCP SDK does not implement per-request `notifications/cancelled`, so this does not promise MCP-level request cancellation. SIGTERM stops the process; an input EOF can wait for an active fetch to finish, bounded by its 20-second timeout.

The [website bundle](https://spacetraders.io/SpaceTraders.json) can differ from the API-hosted document even when both report API 2.3.0. Schema retrieval does not establish live game behavior. See the [official OpenAPI guide](https://docs.spacetraders.io/api-guide/open-api-spec) for the website's generation workflow.

## Connect clients

Replace `/absolute/path` with your checkout directory. No environment variables are needed.

For Codex, add to `~/.codex/config.toml`:

```toml
[mcp_servers.spacetraders-docs]
command = "/absolute/path/spacetraders-mcp/spacetraders-mcp"
args = ["--docs-only"]
```

For Claude Code:

```sh
claude mcp add --scope user --transport stdio spacetraders-docs -- /absolute/path/spacetraders-mcp/spacetraders-mcp --docs-only
```

Open a fresh client session after configuration if the existing session does not reload its MCP catalog. Rebuild the binary after code changes and reconnect the client to use it.

## Verify

```sh
go test -race ./pkg/docs
go build -o spacetraders-mcp .
python3 scripts/check-documentation.py ./spacetraders-mcp
python3 scripts/check-documentation.py ./spacetraders-mcp --live
python3 -O scripts/check-documentation.py ./spacetraders-mcp --exhaustive
python3 -m unittest discover -s scripts -p 'test_*.py'
```

The first smoke check tests initialization, the exact documentation tool catalog, an empty resource list, no advertised prompts, invalid-input handling, and rejection of a gameplay tool call. It strips SpaceTraders environment variables and runs in an empty temporary directory. The optional live check retrieves the public documentation and exercises search, operation details, and schema lookup. `--exhaustive` compares every operation and schema with an independent fetch of the fixed official source, with redirects rejected and the same 8 MiB size limit. Its checks remain active under Python optimization. None of these checks calls gameplay endpoints.

Go tests cover mocked failure/caching behavior plus local HTTPS redirect and compressed-size boundaries. Python regression tests check source restrictions and rejection of a deliberately wrong MCP catalog under `-O`. The stdio checker uses selectors on pipes and currently requires macOS or Linux; Go server builds support Windows, but the checker has not been ported to Windows pipes.
