# Troubleshooting Guide

This document provides solutions to common issues you might encounter when using the SpaceTraders MCP Server.

## Tool Error Handling

### Problem: Getting "status: 422" without details when using navigation tools

**Symptoms:**
- Navigation commands fail with status 422
- Error messages lack specific details about what went wrong

**Cause:**
The SpaceTraders API returns a 422 status code when request parameters don't meet the API's requirements, but the specific validation errors aren't always clear.

**Solutions:**

1. **Check ship status before navigation:**
   ```
   "Show me the current status of GHOST-01"
   ```
   Ensure the ship is in the correct state (orbiting vs. docked) for the operation.

2. **Verify waypoint symbols:**
   - Ensure waypoint symbols are spelled correctly
   - Check that the destination waypoint exists in the system
   - Use the system overview tool to confirm available waypoints

3. **Check fuel levels:**
   ```
   "Check fuel levels for GHOST-01"
   ```
   Insufficient fuel is a common cause of navigation failures.

4. **Verify ship capabilities:**
   - Some operations require specific ship equipment (warp drives, mining equipment)
   - Check ship specifications before attempting operations

**Prevention:**
- Always check ship status before navigation
- Use the `current_location` tool to verify ship positions
- Monitor fuel levels regularly

### Problem: Tool calls failing with authentication errors

**Symptoms:**
- "Invalid token" or "Unauthorized" errors
- Tools fail even though resources work

**Solutions:**

1. **Verify token validity:**
   - Check that your SpaceTraders token hasn't expired
   - Test the token directly with the SpaceTraders API

2. **Check environment variables:**
   ```bash
   echo $SPACETRADERS_TOKEN
   echo $SPACETRADERS_AGENT_SYMBOL
   ```

3. **Restart the MCP server:**
   - Restart Claude Desktop to refresh the connection
   - Ensure the server picks up new environment variables

### Problem: Tools report success but no action is taken

**Symptoms:**
- Tool calls return success messages
- No actual change occurs in the game

**Solutions:**

1. **Check API rate limits:**
   - SpaceTraders API has rate limits
   - Wait a moment and try again

2. **Verify ship requirements:**
   - Some actions require specific ship states
   - Ensure prerequisites are met before attempting actions

3. **Check for conflicting operations:**
   - Some operations can't be performed simultaneously
   - Ensure no other operations are in progress

## System Exploration and Discovery

### Problem: Claude can't find waypoints with specific facilities

**Symptoms:**
- "No shipyards found" when you know they exist
- System exploration returns incomplete data

**Solutions:**

1. **Use the correct system symbol:**
   ```
   "Get an overview of system X1-DF55"
   ```
   Ensure you're using the exact system symbol.

2. **Check waypoint traits:**
   ```
   "Find waypoints with SHIPYARD trait in X1-DF55"
   ```
   Use the `find_waypoints` tool with specific trait filters.

3. **Refresh system data:**
   - System data might be cached
   - Try accessing the system resource directly:
   ```
   spacetraders://systems/X1-DF55/waypoints
   ```

4. **Verify facility availability:**
   - Not all waypoints have the facilities you're looking for
   - Some facilities might be temporarily unavailable

**Prevention:**
- Always verify system symbols before exploration
- Use the system overview tool for comprehensive system information
- Cross-reference with official SpaceTraders documentation

### Problem: System exploration returns empty or minimal data

**Symptoms:**
- System overviews show few or no waypoints
- Missing facility information

**Solutions:**

1. **Check system accessibility:**
   - Ensure you have access to the system
   - Some systems might require exploration first

2. **Verify API connectivity:**
   - Test basic API access with agent information
   - Check network connectivity

3. **Use direct resource access:**
   ```
   spacetraders://systems/SYSTEM-SYMBOL/waypoints
   ```

4. **Check for system updates:**
   - SpaceTraders systems can change
   - Refresh your data periodically

## MCP Resource Issues

### Problem: Claude doesn't know about my agent/ships/contracts

**Symptoms:**
- Claude asks for information that should be available
- "I don't have access to your current status" messages

**Solutions:**

1. **Explicitly reference resources:**
   ```
   "Check my agent status at spacetraders://agent/info"
   "Show my ships from spacetraders://ships/list"
   ```

2. **Verify MCP server connection:**
   - Restart Claude Desktop
   - Check Claude Desktop's MCP server logs
   - Ensure the server is running

3. **Use status commands:**
   ```
   "Run a status_check to see my current situation"
   "Get my status summary"
   ```

**Prevention:**
- Reference resources explicitly when starting conversations
- Use the status_check prompt regularly
- Keep Claude Desktop updated

### Problem: "Resource not found" errors

**Symptoms:**
- Specific resources return 404 errors
- URIs that should work don't resolve

**Solutions:**

1. **Check resource URI syntax:**
   ```
   # Correct format
   spacetraders://systems/X1-DF55/waypoints
   
   # Common mistakes
   spacetraders://systems/X1-DF55/waypoint  # Missing 's'
   spacetraders://system/X1-DF55/waypoints  # Missing 's'
   ```

2. **Verify parameters exist:**
   - Ensure system symbols exist in your game
   - Check waypoint symbols are spelled correctly
   - Verify ship symbols match your fleet

3. **Test with known good values:**
   - Start with basic resources like agent info
   - Use your headquarters system for testing
   - Verify with your actual ship symbols

### Problem: Debug logs show no resource requests

**Symptoms:**
- MCP server logs don't show resource requests
- Resources seem to be ignored

**Solutions:**

1. **Enable debug logging:**
   ```bash
   export DEBUG=1
   ```

2. **Check Claude Desktop configuration:**
   - Verify the MCP server is properly configured
   - Ensure the server binary path is correct

3. **Test server connectivity:**
   ```bash
   # Test the server directly
   ./spacetraders-mcp
   ```

4. **Restart Claude Desktop completely:**
   - Close all Claude Desktop windows
   - Quit the application entirely
   - Restart and test

## Other Common Issues

### Problem: High memory usage or slow performance

**Solutions:**

1. **Restart the MCP server periodically**
2. **Limit concurrent operations**
3. **Check for memory leaks in debug mode**
4. **Monitor system resources**

### Problem: Inconsistent behavior between sessions

**Solutions:**

1. **Clear any cached data**
2. **Restart Claude Desktop between sessions**
3. **Verify environment variables are persistent**
4. **Check for token expiration**

### Problem: Tools work but resources don't (or vice versa)

**Solutions:**

1. **Check different authentication mechanisms**
2. **Verify both tool and resource endpoints**
3. **Test with minimal examples of each**
4. **Review server logs for different error patterns**

## Server Management

### Restarting the MCP Server

If you need to restart the MCP server:

1. **Close Claude Desktop completely**
2. **Wait a few seconds**
3. **Restart Claude Desktop**
4. **Test basic functionality** with a simple status check

### Checking Server Status

```bash
# Check if the server is running
ps aux | grep spacetraders-mcp

# Test server directly
./spacetraders-mcp --test
```

## Logging

### Built-in Logging Levels

The server supports different logging levels:
- `ERROR`: Critical issues only
- `WARN`: Warnings and errors
- `INFO`: General information (default)
- `DEBUG`: Detailed debugging information

### Log Output Examples

**Normal operation:**
```
INFO: MCP server starting
INFO: Registered resource: agent/info
INFO: Registered tool: navigate_ship
INFO: Server ready
```

**Error conditions:**
```
ERROR: Failed to authenticate with SpaceTraders API
WARN: Rate limit approaching
DEBUG: Processing resource request: spacetraders://ships/list
```

### Log Features

- **Structured logging** with contextual information
- **Request tracing** for debugging MCP protocol issues
- **Performance monitoring** for slow operations
- **Error categorization** for easier troubleshooting

### MCP Client Logging

Claude Desktop also logs MCP interactions. Check Claude Desktop's logs for:
- Connection status
- Resource requests
- Error messages
- Performance metrics

### Viewing Logs

**Real-time monitoring:**
```bash
export DEBUG=1
./spacetraders-mcp 2>&1 | tee server.log
```

**Log analysis:**
```bash
grep ERROR server.log
grep "status: 4" server.log  # HTTP 4xx errors
```

## Getting Help

If you continue to experience issues:

1. **Check the project's GitHub issues** for similar problems
2. **Review the SpaceTraders API documentation** for any changes
3. **Test your setup** with the minimal reproduction steps
4. **Collect debug logs** before reporting issues
5. **Provide specific error messages** and steps to reproduce

### Information to Include When Reporting Issues

- Operating system and version
- Go version
- Claude Desktop version
- SpaceTraders agent details (without tokens)
- Exact error messages
- Steps to reproduce the issue
- Debug logs (sanitized of sensitive information)

### Common Resolution Steps

Before reporting an issue, try these common solutions:

1. **Restart everything** (Claude Desktop, terminal, etc.)
2. **Verify your SpaceTraders account** is active and accessible
3. **Test with a fresh token** if possible
4. **Check for system updates** or changes
5. **Try with different ships/systems** to isolate the problem