# SpaceTraders MCP Server

[![Go Reference](https://pkg.go.dev/badge/github.com/grantmd/spacetraders-mcp.svg)](https://pkg.go.dev/github.com/grantmd/spacetraders-mcp)

A Model Context Protocol (MCP) server that provides seamless integration between any MCP client and the SpaceTraders API, enabling automation, analysis, and discussion about the current state of your SpaceTraders game state.

**Most of this is being "written" via LLM tools as a personal experiment, which is probably unsurprising given the target audience, but I wanted to make that clear upfront.**

## ✨ Features

- **🤖 Intelligent Game Assistant**: Get contextual advice and strategic recommendations
- **📊 Real-time Data Access**: Automatic access to your ships, contracts, and systems
- **🛠️ Automated Actions**: Execute complex operations through natural language
- **🗺️ Smart Exploration**: Intelligent system mapping and facility discovery
- **📈 Strategic Analysis**: Contract optimization and fleet management recommendations
- **🚢 Advanced Ship Management**: Individual ship monitoring with real-time cooldown tracking
- **📦 Contract Workflow**: Complete contract delivery system with incremental goods delivery

## 🚀 Quick Start

### Documentation without a game token

Build with `go build -o spacetraders-mcp .` and launch with `--docs-only` for
`search_docs`, `get_operation`, and `get_schema`. This mode skips game credentials
and exposes only public API documentation tools. See the [documentation tools
guide](docs/documentation.md) for client setup, source freshness, and verification.

### Prerequisites

- A current patched Go release. The module's minimum language version is Go 1.24.4.
- Java 8+ and [OpenAPI Generator](https://openapi-generator.tech/docs/installation) (for `make build` on fresh clone; `brew install openapi-generator` on macOS)
- Any MCP Client
- SpaceTraders API token ([Get one here](https://spacetraders.io))

### Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/grantmd/spacetraders-mcp.git
   cd spacetraders-mcp
   ```

2. **Build the server** (auto-generates the SpaceTraders API client if needed):
   ```bash
   make build
   ```

3. **Set up your SpaceTraders credentials:**
   ```bash
   export SPACETRADERS_TOKEN=your_token_here
   ```

4. **Configure Claude Desktop (or other client):**

   Edit `~/Library/Application Support/Claude/claude_desktop_config.json` (Claude macOS):
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

5. **Restart your client** and start exploring!

### First Steps

Once configured, try these commands:

- **"Show me my current SpaceTraders status"** - Get a complete overview
- **"What systems should I explore next?"** - Get strategic recommendations
- **"Help me optimize my fleet"** - Analyze and improve your ships
- **"Find the best trading opportunities"** - Discover profitable routes

## 📖 Documentation

Detailed documentation is available in the `docs/` folder:

- **[Resources](docs/resources.md)** - Available data resources and how to use them
- **[Ship Resources](docs/SHIP_RESOURCES.md)** - Individual ship monitoring and cooldown tracking
- **[Tools](docs/tools.md)** - Actions you can perform through Claude
- **[Prompts](docs/prompts.md)** - Intelligent assistance for common tasks
- **[Integration](docs/integration.md)** - Claude Desktop setup and configuration
- **[MCP Resources Guide](docs/mcp-resources.md)** - Understanding how MCP resources work
- **[Development](docs/development.md)** - Contributing and extending the server
- **[Testing](docs/testing.md)** - Running tests and quality assurance
- **[Troubleshooting](docs/troubleshooting.md)** - Common issues and solutions
- **[API Reference](docs/api-reference.md)** - Complete API documentation
- **[CI/CD](docs/cicd.md)** - Automated workflows and deployment

## 🎮 What You Can Do

### Game Management
- Check agent status, credits, and fleet overview
- Monitor ship locations, cargo, and fuel levels
- **Real-time ship cooldown monitoring** - Know exactly when ships are ready for actions
- **Individual ship analysis** - Comprehensive ship status with operational recommendations
- Review and manage contracts
- **Incremental contract delivery** - Deliver goods to contracts in multiple trips
- Track system exploration progress

### Strategic Operations
- Navigate ships between waypoints and systems
- Extract resources and manage cargo
- **Smart cooldown management** - Plan actions around ship availability
- **Fleet coordination** - Monitor multiple ships' operational status
- Find optimal trading routes and opportunities
- Purchase and configure new ships
- **Complete contract workflows** - Accept, deliver, and fulfill contracts efficiently

### Intelligent Assistance
- Get recommendations based on your current situation
- **Operational status analysis** - Understand what each ship can do right now
- **Capability detection** - Automatic identification of ship roles (mining, trading, etc.)
- Analyze systems for facilities and resources
- Optimize contract selection and completion strategies
- **Action planning** - Get suggestions based on ship cooldowns and capabilities
- Plan fleet composition and expansion

## 🔗 Example Workflows

**Contract Delivery Workflow:**
```
1. "Check my contracts" - Review available and accepted contracts
2. "Show me ship TRADER-1 status" - Check ship location and cargo capacity
3. "Navigate TRADER-1 to the delivery location"
4. "Buy the required goods for the contract"
5. "Deliver goods to contract CONTRACT_123" - Incremental delivery
6. "Check cooldown for TRADER-1" - Monitor when ready for next action
7. "Fulfill the contract" - Complete and claim rewards
```

**Fleet Cooldown Management:**
```
1. "Show me all ship cooldowns" - Monitor fleet availability
2. "Which ships are ready for mining?" - Find available extraction ships
3. "Check ship MINER-1 detailed status" - Comprehensive operational analysis
4. "Plan next actions based on cooldowns" - Strategic fleet coordination
```

**System Exploration:**
```
"Explore system X1-DF55 and tell me what facilities are available"
```

**Fleet Management:**
```
"Where are all my ships and what should I do with them?"
```

**Trading Strategy:**
```
"Analyze my current cargo and find the best places to sell"
```

**Contract Planning:**
```
"Review my contracts and suggest the most profitable completion order"
```

## 🆘 Getting Help

- **First time setup?** Check the [Integration Guide](docs/integration.md)
- **Having issues?** See the [Troubleshooting Guide](docs/troubleshooting.md)
- **Want to contribute?** Read the [Development Guide](docs/development.md)
- **Found a bug?** [Open an issue](https://github.com/grantmd/spacetraders-mcp/issues)

## 📜 License

Apache License - see [LICENSE](LICENSE) for details.

## 🤝 Contributing

Contributions are welcome! Please see our [Development Guide](docs/development.md) for details on how to get started.
