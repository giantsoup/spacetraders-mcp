# Integration with Claude Desktop

This document explains how to integrate the SpaceTraders MCP Server with Claude Desktop for an enhanced SpaceTraders gaming experience.

## Prerequisites

- Claude Desktop installed on your system
- Go 1.21 or higher installed
- A SpaceTraders API token

## Installation

### 1. Clone and Build

```bash
git clone https://github.com/grantmd/spacetraders-mcp.git
cd spacetraders-mcp
go build -o spacetraders-mcp
```

### 2. Set Up Environment Variables

Create a `.env` file or set environment variables:

```bash
export SPACETRADERS_TOKEN=your_token_here
export SPACETRADERS_AGENT_SYMBOL=your_agent_symbol
```

## Claude Desktop Configuration

### macOS

Edit your Claude Desktop configuration file at:
`~/Library/Application Support/Claude/claude_desktop_config.json`

Add the SpaceTraders MCP server configuration:

```json
{
  "mcpServers": {
    "spacetraders": {
      "command": "/path/to/spacetraders-mcp/spacetraders-mcp",
      "args": []
    }
  }
}
```

**Important:** Replace `/path/to/spacetraders-mcp/spacetraders-mcp` with the actual full path to your compiled binary.

### Windows

Edit your Claude Desktop configuration file at:
`%APPDATA%\Claude\claude_desktop_config.json`

Use the same JSON configuration as above, but ensure the path uses Windows-style paths:

```json
{
  "mcpServers": {
    "spacetraders": {
      "command": "C:\\path\\to\\spacetraders-mcp\\spacetraders-mcp.exe",
      "args": []
    }
  }
}
```

### Linux

Edit your Claude Desktop configuration file at:
`~/.config/Claude/claude_desktop_config.json`

Use the same JSON configuration as macOS.

## Environment Variables in Configuration

If you prefer to set environment variables directly in the Claude Desktop configuration:

```json
{
  "mcpServers": {
    "spacetraders": {
      "command": "/path/to/spacetraders-mcp/spacetraders-mcp",
      "args": [],
      "env": {
        "SPACETRADERS_TOKEN": "your_token_here",
        "SPACETRADERS_AGENT_SYMBOL": "your_agent_symbol"
      }
    }
  }
}
```

## Verification

After setting up the configuration:

1. **Restart Claude Desktop** completely
2. **Open a new conversation**
3. **Test the integration** by asking Claude:
   - "Show me my SpaceTraders agent status"
   - "List my ships"
   - "What resources are available?"

You should see Claude automatically accessing your SpaceTraders data through the MCP server.

## Troubleshooting Integration Issues

### Server Not Starting

**Symptoms:** Claude doesn't recognize SpaceTraders commands or resources

**Solutions:**
1. Verify the path to the binary is correct and absolute
2. Ensure the binary has execute permissions (`chmod +x spacetraders-mcp`)
3. Check that environment variables are set correctly
4. Restart Claude Desktop completely

### Authentication Errors

**Symptoms:** "Invalid token" or authentication-related errors

**Solutions:**
1. Verify your SpaceTraders token is correct
2. Ensure the token hasn't expired
3. Check that the agent symbol matches your token
4. Test the token directly with the SpaceTraders API

### Resource Access Issues

**Symptoms:** "Resource not found" errors when accessing `spacetraders://` URIs

**Solutions:**
1. Ensure the MCP server is running (check Claude Desktop logs)
2. Verify your agent has ships/contracts (new agents might not have data)
3. Check that system/waypoint symbols exist and are spelled correctly

## Advanced Configuration

### Custom Binary Location

You can place the binary anywhere on your system. Just ensure:
- The path in the configuration is absolute
- The binary has execute permissions
- The directory is accessible to Claude Desktop

### Multiple Agents

To use multiple SpaceTraders agents, create separate MCP server configurations:

```json
{
  "mcpServers": {
    "spacetraders-agent1": {
      "command": "/path/to/spacetraders-mcp",
      "env": {
        "SPACETRADERS_TOKEN": "token1",
        "SPACETRADERS_AGENT_SYMBOL": "AGENT1"
      }
    },
    "spacetraders-agent2": {
      "command": "/path/to/spacetraders-mcp",
      "env": {
        "SPACETRADERS_TOKEN": "token2",
        "SPACETRADERS_AGENT_SYMBOL": "AGENT2"
      }
    }
  }
}
```

### Development Mode

For development, you can run the server directly from source:

```json
{
  "mcpServers": {
    "spacetraders": {
      "command": "go",
      "args": ["run", "main.go"],
      "cwd": "/path/to/spacetraders-mcp"
    }
  }
}
```

## Best Practices

1. **Use absolute paths** in your configuration
2. **Set environment variables** rather than hardcoding tokens
3. **Test thoroughly** after any configuration changes
4. **Keep tokens secure** - never commit them to version control
5. **Monitor logs** for troubleshooting integration issues

## Integration Features

Once integrated, you'll have access to:

- **Automatic data fetching** via `spacetraders://` resources
- **Action execution** through natural language commands
- **Intelligent prompts** for common SpaceTraders tasks
- **Real-time game state** without manual API calls
- **Contextual assistance** based on your current situation

## Getting Help

If you encounter integration issues:

1. Check the [Troubleshooting guide](troubleshooting.md)
2. Review Claude Desktop's MCP documentation
3. Verify your SpaceTraders API access independently
4. Check the project's GitHub issues for similar problems
