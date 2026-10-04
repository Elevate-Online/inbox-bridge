// Package mcpserver exposes Gmail read tools over the MCP stdio transport,
// backed by whichever Google accounts have been connected via the setup
// wizard.
package mcpserver

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"google-multi-auth/internal/gmailapi"
	"google-multi-auth/internal/oauthflow"
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

// AddAccountInput is empty: add_account takes no arguments. Which account
// gets connected is whichever one the user picks on Google's sign-in page.
type AddAccountInput struct{}

// AddAccountOutput tells Claude what happened and what to tell the user.
type AddAccountOutput struct {
	Status    string `json:"status" jsonschema:"What happened, in plain language, to relay to the user"`
	SignInURL string `json:"signInUrl,omitempty" jsonschema:"Google sign-in link, in case the browser did not open by itself"`
}

// RemoveAccountInput names the account to disconnect.
type RemoveAccountInput struct {
	Account string `json:"account" jsonschema:"Email address of the connected Gmail account to disconnect"`
}

// RemoveAccountOutput reports whether the account was found and removed.
type RemoveAccountOutput struct {
	Status string `json:"status" jsonschema:"What happened, in plain language, to relay to the user"`
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
		return "No accounts connected yet -- use add_account to connect one."
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

	mcp.AddTool(server, &mcp.Tool{
		Name: "add_account",
		Description: "Connect another Gmail account. Opens a Google sign-in page in the user's browser and returns straight away; " +
			"the account is connected once the user finishes signing in there. Tell the user to pick the account in the browser, " +
			"then call list_accounts to confirm it appears. Use this whenever the user asks to add, connect or link a Gmail account.",
	}, addAccountHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_account",
		Description: "Disconnect one Gmail account and delete its saved sign-in from this computer. " + blurb,
	}, removeAccountHandler)
}

// addAccountHandler starts Google's sign-in in the user's browser and
// finishes it in the background. It returns immediately rather than holding
// the tool call open for up to three minutes, which a client may time out.
func addAccountHandler(_ context.Context, _ *mcp.CallToolRequest, _ AddAccountInput) (*mcp.CallToolResult, AddAccountOutput, error) {
	creds, err := oauthflow.LoadClientCredentials()
	if err != nil {
		return nil, AddAccountOutput{}, err
	}
	if creds == nil {
		return nil, AddAccountOutput{}, fmt.Errorf("no Google Client ID and Client secret are set. " +
			"In Claude Desktop, open Settings, then Extensions, then google-multi-auth, and paste them in")
	}

	pending, err := oauthflow.StartAuthFlow(creds)
	if err != nil {
		return nil, AddAccountOutput{}, err
	}

	go func() {
		email, tok, err := pending.Wait()
		if err != nil {
			log.Printf("add_account: %v", err)
			return
		}
		if err := tokenstore.SaveToken(email, tok); err != nil {
			log.Printf("add_account: saving token for %s: %v", email, err)
			return
		}
		log.Printf("add_account: connected %s", email)
	}()

	oauthflow.OpenBrowser(pending.URL)

	return nil, AddAccountOutput{
		Status: "A Google sign-in page has opened in the browser. Pick the Gmail account to connect and allow read access. " +
			"If Google warns that the app is unverified or in testing, that is expected for this tool. " +
			"The sign-in page stays valid for three minutes. If no browser window opened, use the sign-in link.",
		SignInURL: pending.URL,
	}, nil
}

func removeAccountHandler(_ context.Context, _ *mcp.CallToolRequest, input RemoveAccountInput) (*mcp.CallToolResult, RemoveAccountOutput, error) {
	found, err := tokenstore.RemoveAccount(input.Account)
	if err != nil {
		return nil, RemoveAccountOutput{}, err
	}
	if !found {
		return nil, RemoveAccountOutput{Status: fmt.Sprintf("%s is not connected.", input.Account)}, nil
	}
	return nil, RemoveAccountOutput{Status: fmt.Sprintf("Disconnected %s and deleted its saved sign-in.", input.Account)}, nil
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
