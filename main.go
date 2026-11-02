package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"spacetraders-mcp/pkg/client"
	"spacetraders-mcp/pkg/config"
	"spacetraders-mcp/pkg/docs"
	"spacetraders-mcp/pkg/logging"
	"spacetraders-mcp/pkg/resources"
	"spacetraders-mcp/pkg/tools"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Modified to support token-free documentation tools and documentation-only startup.
	docsOnly := flag.Bool("docs-only", false, "Expose only public documentation tools; do not load game credentials")
	tokenFile := flag.String("token-file", "", "Read the agent token from an owner-only secret file")
	tokenCommand := flag.String("token-command", "", "Read the agent token from a secret helper, supplied as a JSON argv array")
	flag.Parse()
	// Set up error logging
	errorLogger := log.New(os.Stderr, "[ERROR] ", log.LstdFlags|log.Lshortfile)

	// Create MCP server with resource and logging capabilities
	s := server.NewMCPServer(
		"SpaceTraders MCP Server",
		"1.0.0",
		server.WithResourceCapabilities(false, false), // subscribe=false, listChanged=false
		server.WithLogging(),                          // Enable MCP logging support
	)
	docs.Register(s, docs.NewCatalog())
	if *docsOnly {
		serve(s, errorLogger)
		return
	}

	// Gameplay mode retains the existing credential requirement.
	cfg, err := config.Load(config.Options{TokenFile: *tokenFile, TokenCommand: *tokenCommand})
	if err != nil {
		errorLogger.Printf("Configuration error: %v", err)
		os.Exit(1)
	}
	for _, warning := range cfg.Warnings {
		fmt.Fprintln(os.Stderr, "Credential fallback:", warning)
	}
	spacetradersClient := client.NewClient(cfg.SpaceTradersAPIToken)

	// Create application logger
	appLogger := logging.NewLogger(s)

	// Add logging support - send log messages to MCP client
	s.AddNotificationHandler("logging/setLevel", func(ctx context.Context, notification mcp.JSONRPCNotification) {
		errorLogger.Printf("Client requested logging level change: %+v", notification)
	})

	// Note: MCP framework handles resources/list and tools/list automatically
	// To see these calls, you would need to monitor the stdio communication directly
	appLogger.Debug("MCP server configured - resources/list and tools/list calls will be handled automatically")

	appLogger.Info("Starting SpaceTraders MCP Server")
	appLogger.Info("Gameplay credential source: %s", cfg.TokenSource)

	// Register all resources
	resourceRegistry := resources.NewRegistry(spacetradersClient, appLogger)
	resourceRegistry.RegisterWithServer(s)

	// Register all tools (when we have them)
	toolRegistry := tools.NewRegistry(spacetradersClient, appLogger)
	toolRegistry.RegisterWithServer(s)

	// Register prompts to help guide user interactions
	s.AddPrompt(mcp.Prompt{
		Name:        "status_check",
		Description: "Get comprehensive status of your SpaceTraders agent including ships, contracts, and opportunities",
		Arguments: []mcp.PromptArgument{
			{
				Name:        "detail_level",
				Description: "Level of detail (basic, detailed, full)",
				Required:    false,
			},
		},
	}, func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		detailLevel := "basic"
		if request.Params.Arguments != nil {
			if level, exists := request.Params.Arguments["detail_level"]; exists {
				detailLevel = level
			}
		}

		prompt := "I'd like to check my SpaceTraders status. Please:\n\n"
		prompt += "1. Use the get_status_summary tool to get my current agent status\n"
		prompt += "2. Read my ships list from spacetraders://ships/list\n"
		prompt += "3. Read my contracts from spacetraders://contracts/list\n"

		if detailLevel == "detailed" || detailLevel == "full" {
			prompt += "4. If I have ships in different systems, show waypoints for those systems\n"
			prompt += "5. Check for any shipyards or marketplaces at my current locations\n"
		}

		if detailLevel == "full" {
			prompt += "6. Suggest 3-5 concrete next actions based on my current situation\n"
			prompt += "7. Identify any immediate opportunities (profitable contracts, good trade routes, etc.)\n"
		}

		prompt += "\nPlease provide a clear summary and actionable recommendations."

		return &mcp.GetPromptResult{
			Description: "Comprehensive SpaceTraders status check",
			Messages: []mcp.PromptMessage{
				{
					Role: "user",
					Content: mcp.TextContent{
						Type: "text",
						Text: prompt,
					},
				},
			},
		}, nil
	})

	s.AddPrompt(mcp.Prompt{
		Name:        "explore_system",
		Description: "Explore a specific system to find trading opportunities, shipyards, and points of interest",
		Arguments: []mcp.PromptArgument{
			{
				Name:        "system_symbol",
				Description: "System symbol to explore (e.g., X1-FM66)",
				Required:    true,
			},
		},
	}, func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		systemSymbol := ""
		if request.Params.Arguments != nil {
			if system, exists := request.Params.Arguments["system_symbol"]; exists {
				systemSymbol = system
			}
		}

		if systemSymbol == "" {
			systemSymbol = "{SYSTEM_SYMBOL}"
		}

		prompt := fmt.Sprintf("I want to explore system %s. Please:\n\n", systemSymbol)
		prompt += fmt.Sprintf("1. Read the waypoints in this system from spacetraders://systems/%s/waypoints\n", systemSymbol)
		prompt += "2. Identify which waypoints have:\n"
		prompt += "   - Marketplaces (for trading)\n"
		prompt += "   - Shipyards (for buying ships)\n"
		prompt += "   - Mining sites (for resource extraction)\n"
		prompt += "   - Other interesting traits\n"
		prompt += "3. For any shipyards found, check what ships are available\n"
		prompt += "4. Based on my current ships and credits, suggest:\n"
		prompt += "   - Best trading opportunities\n"
		prompt += "   - Whether I should buy new ships\n"
		prompt += "   - Optimal travel routes within the system\n"
		prompt += "\nProvide a strategic analysis of this system's potential."

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Explore system %s for opportunities", systemSymbol),
			Messages: []mcp.PromptMessage{
				{
					Role: "user",
					Content: mcp.TextContent{
						Type: "text",
						Text: prompt,
					},
				},
			},
		}, nil
	})

	s.AddPrompt(mcp.Prompt{
		Name:        "contract_strategy",
		Description: "Analyze available contracts and suggest the best ones to accept based on current capabilities",
		Arguments:   []mcp.PromptArgument{},
	}, func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		prompt := "Help me develop a contract strategy. Please:\n\n"
		prompt += "1. Read my current contracts from spacetraders://contracts/list\n"
		prompt += "2. Get my current status using get_status_summary\n"
		prompt += "3. For each available contract, analyze:\n"
		prompt += "   - Profitability (payment vs effort required)\n"
		prompt += "   - Feasibility (do I have ships/cargo space?)\n"
		prompt += "   - Location convenience (are delivery points near my ships?)\n"
		prompt += "   - Time constraints (can I complete before deadline?)\n"
		prompt += "4. Recommend which contracts to accept and why\n"
		prompt += "5. If I need to move ships or buy cargo space, provide a plan\n"
		prompt += "\nFocus on maximizing profit while minimizing risk and travel time."

		return &mcp.GetPromptResult{
			Description: "Strategic contract analysis and recommendations",
			Messages: []mcp.PromptMessage{
				{
					Role: "user",
					Content: mcp.TextContent{
						Type: "text",
						Text: prompt,
					},
				},
			},
		}, nil
	})

	s.AddPrompt(mcp.Prompt{
		Name:        "fleet_optimization",
		Description: "Analyze current fleet and suggest optimizations for better efficiency and profit",
		Arguments:   []mcp.PromptArgument{},
	}, func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		prompt := "Help me optimize my fleet. Please:\n\n"
		prompt += "1. Get my current status and ship details\n"
		prompt += "2. Read my ships list from spacetraders://ships/list\n"
		prompt += "3. Analyze my current fleet composition:\n"
		prompt += "   - Ship types and roles\n"
		prompt += "   - Cargo capacity utilization\n"
		prompt += "   - Geographic distribution\n"
		prompt += "   - Fuel efficiency\n"
		prompt += "4. Check shipyards in systems where I have ships\n"
		prompt += "5. Recommend fleet improvements:\n"
		prompt += "   - Should I buy additional ships?\n"
		prompt += "   - Are there better ship types for my activities?\n"
		prompt += "   - Should I relocate ships to different systems?\n"
		prompt += "   - Any upgrades or modifications needed?\n"
		prompt += "\nProvide a strategic fleet development plan with cost-benefit analysis."

		return &mcp.GetPromptResult{
			Description: "Fleet composition analysis and optimization recommendations",
			Messages: []mcp.PromptMessage{
				{
					Role: "user",
					Content: mcp.TextContent{
						Type: "text",
						Text: prompt,
					},
				},
			},
		}, nil
	})

	appLogger.Info("Server initialization complete")
	serve(s, errorLogger)
}

func serve(s *server.MCPServer, errorLogger *log.Logger) {
	// Start the stdio server with error logging (ServeStdio already handles signals gracefully)
	if err := server.ServeStdio(s, server.WithErrorLogger(errorLogger)); err != nil && err != context.Canceled {
		errorLogger.Printf("Server error: %v", err)
	}
}
