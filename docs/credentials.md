# Gameplay credentials

Normal mode resolves the first usable agent token at startup, in this order:

1. A secret helper configured with `--token-command` or `SPACETRADERS_TOKEN_COMMAND`.
2. A secret file configured with `--token-file` or `SPACETRADERS_TOKEN_FILE`.
3. `SPACETRADERS_API_TOKEN` in the process environment.
4. `SPACETRADERS_TOKEN` in the process environment.
5. `SPACETRADERS_API_TOKEN`, then `SPACETRADERS_TOKEN`, in the working directory's `.env`.

Flags override the corresponding environment reference. Empty or missing sources are skipped. Failed, unreadable, oversized, or invalid sources produce sanitized diagnostics and the loader tries the next source. If none works, startup exits with configuration guidance. Only successful local loading is established at this point; the API validates the token when a gameplay request runs.

The selected token stays in memory for that process. The server does not change sources after an API rejection, replay gameplay writes, or silently refresh into a different account. Reconnect after rotating credentials. Use an **agent token** for the intended account; an account token used for registration is not interchangeable with an agent token. The loader checks header-safe syntax, not token authenticity or account ownership.

`--docs-only` skips all credential loading, including helper execution and file access. It still exposes exactly `search_docs`, `get_operation`, and `get_schema`.

## Secret helpers

Supply a JSON array of an executable and arguments. The helper writes only the token to stdout. The MCP process captures it directly; do not run the helper in a terminal or log its output. Arguments and MCP settings must contain references to secrets, never secret values.

For example, a macOS Keychain item that you already created can be read with:

```json
{
  "command": "/absolute/path/spacetraders-mcp/spacetraders-mcp",
  "args": ["--token-command", "[\"/usr/bin/security\",\"find-generic-password\",\"-s\",\"spacetraders\",\"-a\",\"YOUR_AGENT_SYMBOL\",\"-w\"]"]
}
```

A password-manager CLI or a private credential adapter can be configured the same way. Set up its authentication separately. The MCP does not enumerate vaults, prompt interactively, create Keychain items, or scan the filesystem. The configured executable is trusted local code with your OS permissions, so use a trusted absolute executable path and protect helper scripts from modification.

Helpers run without a shell or stdin, with a five-second timeout and a 16 KiB output limit. Stderr is discarded; paths, arguments, subprocess errors, and output are omitted from MCP diagnostics. `--token-command` is never read from a working-directory `.env`. This prevents a repository's `.env` from authorizing program execution. Go's [`os/exec` documentation](https://pkg.go.dev/os/exec) explains its direct argument handling and command cancellation.

## Existing local Laravel account

The optional `scripts/read-laravel-token.php` adapter can reuse the encrypted credential saved by the companion Laravel app. It requires that app's installed Composer dependencies, PHP with PDO SQLite, and the matching application encryption key. The Go server has no PHP or Laravel dependency when this adapter is not configured.

Configure your MCP client with paths to your PHP executable, this adapter, and the app:

```json
{
  "command": "/absolute/path/spacetraders-mcp/spacetraders-mcp",
  "args": ["--token-command", "[\"/absolute/path/php\",\"/absolute/path/spacetraders-mcp/scripts/read-laravel-token.php\",\"/absolute/path/spacetraders\"]"]
}
```

The adapter reads cached Laravel configuration when present, otherwise `.env`. It opens the existing SQLite database with `mode=ro` and `PRAGMA query_only=ON`, selects only connected agents with version-1 encrypted tokens, and decrypts in memory. It does not boot the app, synchronize state, register agents, or modify the database. Previous encryption keys configured by Laravel are supported.

Exactly one eligible agent must exist. If several exist, append the desired numeric local agent ID as a fourth element of the JSON command array. Reset agents, legacy tokens, missing databases, unsupported database drivers, and unreadable credentials fail safely, allowing the next configured source. No default app path or account is guessed.

## Secret files

`--token-file /absolute/path/agent-token` reads one plain token, optionally ending with a newline. On macOS and Linux the file must be owner-only, such as mode `0600` or `0400`. Use the OS ACL to restrict access on Windows, where this POSIX permission check is not available. Symlinks are permitted only when their target is a regular file with the required permissions. Directories and nonregular files are rejected.

Protect the parent directory and keep secrets outside the checkout. The server never creates a token file or copies its token into `.env`, MCP settings, or another file. File and helper output are limited to 16 KiB. Internal whitespace, control characters, and non-ASCII token content are rejected before building the Authorization header.

## Verification

Run `go test -race -timeout 60s ./pkg/config` for source precedence, failure fallback, secret redaction, file boundaries, command timeouts, and literal argument handling. The normal Go suite uses synthetic credentials for these cases.

The PHP adapter tests are opt-in and create disposable encrypted SQLite fixtures:

```sh
SPACETRADERS_TEST_PHP=/absolute/path/php \
SPACETRADERS_TEST_LARAVEL_AUTOLOAD=/absolute/path/spacetraders/vendor/autoload.php \
python3 -m unittest discover -s scripts -p 'test_read_laravel_token.py'
```

These tests cover account ambiguity, reset exclusion, key rotation, cached configuration, database read-only behavior, and secret-free error messages. Actual authenticated API reads require a separate, explicitly authorized check.
