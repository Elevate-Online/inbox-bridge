// Package cli implements the google-multi-auth command-line subcommands:
// interactive setup, listing connected accounts, and removing one.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"google-multi-auth/internal/desktopconfig"
	"google-multi-auth/internal/oauthflow"
	"google-multi-auth/internal/tokenstore"
)

// Setup walks the user through connecting one or more Gmail accounts and
// registers this binary with Claude Desktop.
func Setup() error {
	creds, err := ensureClientCredentials()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		email, tok, err := oauthflow.RunAuthFlow(creds)
		if err != nil {
			return fmt.Errorf("connecting Gmail account: %w", err)
		}
		if err := tokenstore.SaveToken(email, tok); err != nil {
			return fmt.Errorf("saving token for %s: %w", email, err)
		}
		fmt.Printf("Connected: %s\n", email)

		fmt.Print("Add another Gmail account? (y/N) ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(answer)
		if !strings.HasPrefix(strings.ToLower(answer), "y") {
			break
		}
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving this binary's path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(binaryPath); err == nil {
		binaryPath = resolved
	}

	if err := desktopconfig.Register(binaryPath); err != nil {
		return fmt.Errorf("registering with Claude Desktop: %w", err)
	}

	cfgPath, err := desktopconfig.ConfigPath()
	if err != nil {
		return fmt.Errorf("resolving Claude Desktop config path: %w", err)
	}
	fmt.Printf("\nRegistered with Claude Desktop at %s\n", cfgPath)
	fmt.Printf("Claude Desktop will run %s\n", binaryPath)
	fmt.Println("Don't move or delete this folder, or Claude Desktop loses the Gmail tools.")
	fmt.Println("If you do move it, run setup again from the new location.")
	fmt.Println()
	fmt.Println("Restart Claude Desktop to load the Gmail tools.")
	fmt.Println()
	fmt.Println("Note: while your Google Cloud OAuth app is in \"Testing\" publishing status,")
	fmt.Println("Google expires sign-ins after 7 days and you'll need to run setup again to")
	fmt.Println("reconnect. Switching the app's OAuth consent screen to \"In production\"")
	fmt.Println("removes that limit, but the app stays unverified: each sign-in shows")
	fmt.Println("\"Google hasn't verified this app\", and you continue with Advanced, then")
	fmt.Println("\"Go to (app name) (unsafe)\". That is expected for a personal OAuth client.")

	return nil
}

// List prints the currently connected account emails, one per line.
func List() error {
	accounts, err := tokenstore.ListAccounts()
	if err != nil {
		return fmt.Errorf("listing connected accounts: %w", err)
	}
	if len(accounts) == 0 {
		fmt.Println("(no accounts connected)")
		return nil
	}
	for _, account := range accounts {
		fmt.Println(account)
	}
	return nil
}

// Remove disconnects one account, deleting its stored token.
func Remove(email string) error {
	found, err := tokenstore.RemoveAccount(email)
	if err != nil {
		return fmt.Errorf("removing account %s: %w", email, err)
	}
	if found {
		fmt.Printf("Removed: %s\n", email)
	} else {
		fmt.Printf("No connected account found for: %s\n", email)
	}
	return nil
}

// readSecret reads a line from stdin without echoing it, when stdin is a
// terminal. When it isn't (e.g. piped/scripted input), it falls back to a
// plain read from reader so non-interactive setups keep working.
func readSecret(reader *bufio.Reader) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return reader.ReadString('\n')
	}
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(secret), nil
}

// ensureClientCredentials returns the stored Google OAuth client
// credentials, prompting the user to create and enter one if none exist
// yet.
func ensureClientCredentials() (*oauthflow.ClientCredentials, error) {
	creds, err := oauthflow.LoadClientCredentials()
	if err != nil {
		return nil, fmt.Errorf("loading OAuth client credentials: %w", err)
	}
	if creds != nil {
		return creds, nil
	}

	fmt.Println("No Google OAuth client is configured yet. Every app that reads Gmail")
	fmt.Println("needs its own client -- this is a one-time, ~2 minute setup:")
	fmt.Println()
	fmt.Println("  1. Create a Google Cloud project: https://console.cloud.google.com/projectcreate")
	fmt.Println("  2. Enable the Gmail API: https://console.cloud.google.com/apis/library/gmail.googleapis.com")
	fmt.Println("  3. Configure the OAuth consent screen as \"External\":")
	fmt.Println("     https://console.cloud.google.com/apis/credentials/consent")
	fmt.Println("     - Add the gmail.readonly scope")
	fmt.Println("     - Add every Gmail address you plan to connect as a test user")
	fmt.Println("  4. Create an OAuth client of type \"Desktop app\":")
	fmt.Println("     https://console.cloud.google.com/apis/credentials")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Client ID: ")
	clientID, _ := reader.ReadString('\n')
	clientID = strings.TrimSpace(clientID)

	fmt.Print("Client Secret: ")
	clientSecret, err := readSecret(reader)
	if err != nil {
		return nil, fmt.Errorf("reading client secret: %w", err)
	}
	clientSecret = strings.TrimSpace(clientSecret)

	newCreds := &oauthflow.ClientCredentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	if err := oauthflow.SaveClientCredentials(newCreds); err != nil {
		return nil, fmt.Errorf("saving OAuth client credentials: %w", err)
	}

	return newCreds, nil
}
