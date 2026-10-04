// Command google-multi-auth is both an MCP stdio server exposing Gmail read
// tools across multiple connected Google accounts, and a CLI for connecting
// those accounts and registering itself with Claude Desktop.
package main

import (
	"fmt"
	"os"

	"google-multi-auth/internal/cli"
	"google-multi-auth/internal/mcpserver"
)

const usage = `Usage: google-multi-auth [command]

Commands:
  (none) | serve   Run the MCP server over stdio (default; used by Claude Desktop)
  setup            Interactively connect Gmail accounts and register with Claude Desktop
  list             List connected Gmail accounts
  remove <email>   Disconnect one Gmail account
  -h, --help       Show this help
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return mcpserver.Serve()
	}

	switch args[0] {
	case "serve":
		return mcpserver.Serve()
	case "setup":
		return cli.Setup()
	case "list":
		return cli.List()
	case "remove":
		if len(args) < 2 {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(1)
		}
		return cli.Remove(args[1])
	case "-h", "--help":
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
	return nil
}
