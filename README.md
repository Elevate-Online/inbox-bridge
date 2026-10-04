# Inbox Bridge

Connect more than one Gmail inbox to Claude Desktop, as a local MCP server.

*Formerly google-multi-auth. Accounts connected under the old name carry
over automatically.*

This is a single, self-contained Go binary. There is nothing to install to
use it -- no Node, no Go, no package manager. Prebuilt binaries for macOS,
Linux, and Windows are attached to each
[GitHub Release](https://github.com/Elevate-Online/inbox-bridge/releases/latest);
only someone changing the code needs Go installed.

## Install in Claude Desktop (Mac and Windows)

No terminal needed. Three steps, about 10 minutes, most of it in Google's
settings.

1. **Create your Google sign-in client.** Google requires every app that
   reads Gmail to have its own. It's free:
   1. [Create a Google Cloud project](https://console.cloud.google.com/projectcreate). Any name works.
   2. [Enable the Gmail API](https://console.cloud.google.com/apis/library/gmail.googleapis.com) for it.
   3. [Set up the OAuth consent screen](https://console.cloud.google.com/apis/credentials/consent)
      as External, add the `gmail.readonly` scope, and add **every Gmail
      address you plan to connect** as a test user.
   4. [Create an OAuth client](https://console.cloud.google.com/apis/credentials)
      of type **Desktop app**. Keep the Client ID and Client secret.
2. **Install the extension.** Download
   [`inbox-bridge.mcpb`](https://github.com/Elevate-Online/inbox-bridge/releases/latest/download/inbox-bridge.mcpb)
   and double-click it. Claude Desktop opens an install dialog for Inbox Bridge: click
   Install and paste in the Client ID and Client secret.
3. **Connect your inboxes.** In a new chat, ask Claude to "add my Gmail
   account." A Google sign-in page opens in your browser; pick the account
   and allow read access. Repeat for each inbox.

Then ask something like "Search my work inbox for unread mail from this
week." To disconnect an inbox, ask Claude to remove it.

To change the Client ID or secret later: Claude Desktop, Settings,
Extensions, Inbox Bridge.

## Manual setup (terminal)

For Linux, or if you'd rather register the server yourself.

1. Download the zip for your machine from the
   [latest release](https://github.com/Elevate-Online/inbox-bridge/releases/latest):

   | Machine | Zip |
   |---|---|
   | Any Mac (Apple silicon or Intel) | `inbox-bridge-macos.zip` |
   | Windows | `inbox-bridge-windows-amd64.zip` |
   | Linux | `inbox-bridge-linux-amd64.zip` or `-linux-arm64.zip` |

   The single-chip Mac zips (`-darwin-arm64`, `-darwin-amd64`) are there too
   if you want the smaller download.

   `SHA256SUMS` in the same release lets you check the download:
   `shasum -a 256 -c SHA256SUMS --ignore-missing`.

2. Unzip it and move the `inbox-bridge` folder somewhere permanent (your
   home folder is fine, not Downloads). Setup registers the binary's current
   location with Claude Desktop, so if you move or delete the folder later,
   the Gmail tools stop loading until you run setup again from the new
   location.

3. **macOS only:** the binary isn't signed by Apple, so macOS blocks a
   downloaded copy from running. Clear the download flag on the folder:

   ```sh
   xattr -dr com.apple.quarantine ~/inbox-bridge
   ```

   (Adjust the path if you put the folder somewhere else.) If you skip this,
   macOS shows "Apple could not verify inbox-bridge-darwin-arm64 is free
   of malware" with only Done and Move to Trash. Click Done, then open System
   Settings, Privacy & Security, scroll down and click **Open Anyway**.

4. Run setup from inside the folder:

   ```sh
   cd ~/inbox-bridge
   ./setup
   ```

On Windows, open the folder in Command Prompt and run `setup.bat` instead of
`./setup`. If Windows shows "Windows protected your PC," click **More info**,
then **Run anyway**; the binary isn't code-signed.

The setup wizard will:

1. Walk you through creating a free Google OAuth client (one-time, ~2
   minutes -- Google requires every app that reads Gmail to have its own
   client; the wizard prints the exact console links and fields to fill in).
   Add every Gmail address you plan to connect as a test user on the OAuth
   consent screen, or that account's sign-in will be refused.
2. Open your browser to sign in to a Gmail account, and ask if you'd like to
   add another. Repeat for as many inboxes as you want.
3. Register the server with Claude Desktop automatically.

Restart Claude Desktop afterwards to pick up the new tools.

To add or remove accounts later, run `./setup` again (add), or:

```sh
./bin/inbox-bridge-<your-platform> list
./bin/inbox-bridge-<your-platform> remove someone@gmail.com
```

(`./setup` picks the right binary for your platform automatically; the two
commands above need you to name it yourself.)

## Tools

- `add_account` -- connect another Gmail account through a browser sign-in.
- `list_accounts` -- the connected Gmail addresses.
- `search_messages` -- search one account's inbox with Gmail search syntax
  (`from:`, `is:unread`, `in:inbox`, etc.).
- `get_message` -- fetch the full content of one message by ID.
- `remove_account` -- disconnect an account and delete its saved sign-in.

Gmail access is read-only (`gmail.readonly` scope) -- this cannot send,
delete, or modify mail.

## Where things are stored

Everything lives under `~/.config/inbox-bridge/`, permissioned to your
user only:

- `client.json` -- your Google OAuth client ID/secret (manual setup only;
  the extension keeps them in Claude Desktop's settings instead).
- `tokens/<email>.json` -- one refresh token per connected account.

Nothing leaves your machine except direct calls to Google's API.

## Testing vs. In production

Your Google OAuth app starts in **Testing** publishing status. While it is:

- Only the addresses on its test-user list can sign in.
- Google shows a warning that the app is in testing before each sign-in.
- Sign-ins expire after 7 days, and you re-run `./setup` to reconnect.

To stop the 7-day expiry, open your app's OAuth consent screen in the Cloud
Console and change its publishing status to **In production**. The app is
still unverified after that, because `gmail.readonly` is a restricted scope,
so each sign-in shows "Google hasn't verified this app." That is expected for
a personal OAuth client: click **Advanced**, then **Go to (app name)
(unsafe)**. Google also caps an unverified app at 100 users for its lifetime,
which is plenty for one person.

If you connect a Google Workspace account and see "Error 400:
admin_policy_enforced" or an "access blocked" screen, your Workspace admin
restricts third-party apps. Only the admin can allow it.

## Building from source

Only needed if you're changing the code -- end users never need this.

```sh
./build.sh
```

Cross-compiles static binaries for darwin/arm64, darwin/amd64, linux/amd64,
linux/arm64, and windows/amd64 into `bin/`, and packages one release zip per
platform plus `SHA256SUMS` into `dist/`. Both folders are gitignored; the
zips go on a GitHub Release, never in the repo. With `bin/` built, `./setup`
works from a source checkout too.

## License

MIT. See [LICENSE](LICENSE).

---

Built by [Claude Training](https://www.claudetraining.com).
