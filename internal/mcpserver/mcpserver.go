// Package mcpserver exposes Gmail read tools over the MCP stdio transport,
// backed by whichever Google accounts have been connected via the setup
// wizard.
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"google-multi-auth/internal/gmailapi"
	"google-multi-auth/internal/tokenstore"
)

// ListAccountsInput is empty: list_accounts takes no arguments.
type ListAccountsInput struct{}

// ListAccountsOutput reports the currently connected account addresses.
type ListAccountsOutput struct {
	Accounts []string `json:"accounts" jsonschema:"Email addresses of currently connected Gmail accounts"`
}

// SearchMessagesInput describes the search_messages tool's arguments.
type SearchMessagesInput struct {
	Account    string `json:"account" jsonschema:"Email address of the connected Gmail account to search"`
	Query      string `json:"query,omitempty" jsonschema:"Gmail search query syntax, e.g. 'is:unread from:someone@example.com' (default: in:inbox)"`
	MaxResults int64  `json:"maxResults,omitempty" jsonschema:"Maximum number of messages to return, 1-50 (default 10)"`
}

// SearchMessagesOutput wraps the matching message summaries.
type SearchMessagesOutput struct {
	Messages []gmailapi.MessageSummary `json:"messages" jsonschema:"Matching Gmail messages"`
}

// GetMessageInput describes the get_message tool's arguments.
type GetMessageInput struct {
	Account string `json:"account" jsonschema:"Email address of the connected Gmail account that owns the message"`
	ID      string `json:"id" jsonschema:"Gmail message ID, as returned by search_messages"`
}

// GetMessageOutput wraps the full message detail.
type GetMessageOutput struct {
	Message gmailapi.MessageDetail `json:"message" jsonschema:"Full content of the requested message"`
}

// Serve registers the Gmail tools and runs the MCP server on stdio, blocking
// until the client disconnects.
func Serve() error {
	accounts, err := tokenstore.ListAccounts()
	if err != nil {
		return fmt.Errorf("listing connected accounts: %w", err)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "google-multi-auth",
		Version: "v1.0.0",
	}, nil)

	registerTools(server, accounts)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("running MCP server: %w", err)
	}
	return nil
}

func accountsBlurb(accounts []string) string {
	if len(accounts) == 0 {
		return "No accounts connected yet -- run the setup command."
	}
	return fmt.Sprintf("Connected accounts: %s.", strings.Join(accounts, ", "))
}

func registerTools(server *mcp.Server, accounts []string) {
	blurb := accountsBlurb(accounts)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_accounts",
		Description: "List the Gmail accounts currently connected to this server. " + blurb,
	}, listAccountsHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_messages",
		Description: "Search a connected Gmail account's mail using Gmail search syntax and return matching message summaries. " + blurb,
	}, searchMessagesHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_message",
		Description: "Fetch the full content (including body) of one Gmail message by ID from a connected account. " + blurb,
	}, getMessageHandler)
}

func listAccountsHandler(_ context.Context, _ *mcp.CallToolRequest, _ ListAccountsInput) (*mcp.CallToolResult, ListAccountsOutput, error) {
	accounts, err := tokenstore.ListAccounts()
	if err != nil {
		return nil, ListAccountsOutput{}, fmt.Errorf("listing connected accounts: %w", err)
	}
	return nil, ListAccountsOutput{Accounts: accounts}, nil
}

func searchMessagesHandler(_ context.Context, _ *mcp.CallToolRequest, input SearchMessagesInput) (*mcp.CallToolResult, SearchMessagesOutput, error) {
	query := input.Query
	if query == "" {
		query = "in:inbox"
	}

	maxResults := input.MaxResults
	switch {
	case maxResults <= 0:
		maxResults = 10
	case maxResults > 50:
		maxResults = 50
	}

	messages, err := gmailapi.SearchMessages(input.Account, query, maxResults)
	if err != nil {
		// A non-nil error here is turned into a tool-level error result by
		// AddTool's handler wrapper (IsError + the message as text content),
		// not a transport-level failure -- the server process keeps running.
		return nil, SearchMessagesOutput{}, err
	}

	return nil, SearchMessagesOutput{Messages: messages}, nil
}

func getMessageHandler(_ context.Context, _ *mcp.CallToolRequest, input GetMessageInput) (*mcp.CallToolResult, GetMessageOutput, error) {
	detail, err := gmailapi.GetMessage(input.Account, input.ID)
	if err != nil {
		return nil, GetMessageOutput{}, err
	}

	return nil, GetMessageOutput{Message: *detail}, nil
}
