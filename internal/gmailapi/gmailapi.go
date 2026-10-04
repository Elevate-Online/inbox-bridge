// Package gmailapi builds authenticated Gmail API clients for connected
// accounts and exposes the read operations the MCP tools need.
package gmailapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"inbox-bridge/internal/config"
	"inbox-bridge/internal/oauthflow"
	"inbox-bridge/internal/tokenstore"
)

// MessageSummary is the compact view of a message returned by search.
type MessageSummary struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
	From     string `json:"from"`
	Subject  string `json:"subject"`
	Date     string `json:"date"`
	Snippet  string `json:"snippet"`
}

// MessageDetail is the full view of a single message.
type MessageDetail struct {
	MessageSummary
	To   string `json:"to"`
	Body string `json:"body"`
}

// ClientForAccount builds a Gmail service for the given connected account,
// wiring up a token source that auto-refreshes and persists any refreshed
// token back to disk so the account doesn't silently disconnect once its
// access token expires.
func ClientForAccount(email string) (*gmail.Service, error) {
	creds, err := oauthflow.LoadClientCredentials()
	if err != nil {
		return nil, fmt.Errorf("loading OAuth client credentials: %w", err)
	}
	if creds == nil {
		return nil, fmt.Errorf("no Google OAuth client configured -- run the setup command first")
	}

	tok, err := tokenstore.LoadToken(email)
	if err != nil {
		return nil, fmt.Errorf("loading token for %s: %w", email, err)
	}
	if tok == nil {
		return nil, fmt.Errorf("no connected account %q -- run the setup command to connect it", email)
	}

	conf := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Scopes:       config.Scopes,
		Endpoint:     google.Endpoint,
	}

	ctx := context.Background()
	ts := newPersistingTokenSource(ctx, conf, tok, email)

	srv, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("building Gmail client for %s: %w", email, err)
	}
	return srv, nil
}

// persistingTokenSource wraps a standard oauth2 token source and writes the
// token back to tokenstore whenever it changes (i.e. was refreshed).
type persistingTokenSource struct {
	mu    sync.Mutex
	src   oauth2.TokenSource
	last  string
	email string
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.src.Token()
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if tok.AccessToken != p.last {
		if err := tokenstore.SaveToken(p.email, tok); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to persist refreshed token for %s: %v\n", p.email, err)
		} else {
			p.last = tok.AccessToken
		}
	}
	return tok, nil
}

// newPersistingTokenSource returns a token source that auto-refreshes via
// conf and persists any refreshed token back to disk for email.
func newPersistingTokenSource(ctx context.Context, conf *oauth2.Config, tok *oauth2.Token, email string) oauth2.TokenSource {
	inner := conf.TokenSource(ctx, tok)
	wrapped := &persistingTokenSource{src: inner, last: tok.AccessToken, email: email}
	return oauth2.ReuseTokenSource(tok, wrapped)
}

// SearchMessages runs a Gmail search and returns a summary for each match.
func SearchMessages(email, query string, maxResults int64) ([]MessageSummary, error) {
	srv, err := ClientForAccount(email)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	listResp, err := srv.Users.Messages.List("me").Q(query).MaxResults(maxResults).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("searching messages for %s: %w", email, err)
	}

	summaries := make([]MessageSummary, 0, len(listResp.Messages))
	for _, m := range listResp.Messages {
		msg, err := srv.Users.Messages.Get("me", m.Id).
			Format("metadata").
			MetadataHeaders("From", "Subject", "Date").
			Context(ctx).
			Do()
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: fetching message %s for %s: %v\n", m.Id, email, err)
			continue
		}
		summaries = append(summaries, summaryFromMessage(msg))
	}
	return summaries, nil
}

// GetMessage fetches the full content of one message, including its
// plain-text (or, failing that, HTML) body.
func GetMessage(email, id string) (*MessageDetail, error) {
	srv, err := ClientForAccount(email)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	msg, err := srv.Users.Messages.Get("me", id).Format("full").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("fetching message %s for %s: %w", id, email, err)
	}

	detail := &MessageDetail{
		MessageSummary: summaryFromMessage(msg),
		To:             headerValue(msg.Payload, "To"),
		Body:           extractBody(msg.Payload),
	}
	return detail, nil
}

func summaryFromMessage(msg *gmail.Message) MessageSummary {
	return MessageSummary{
		ID:       msg.Id,
		ThreadID: msg.ThreadId,
		From:     headerValue(msg.Payload, "From"),
		Subject:  headerValue(msg.Payload, "Subject"),
		Date:     headerValue(msg.Payload, "Date"),
		Snippet:  msg.Snippet,
	}
}

func headerValue(payload *gmail.MessagePart, name string) string {
	if payload == nil {
		return ""
	}
	for _, h := range payload.Headers {
		if h.Name == name {
			return h.Value
		}
	}
	return ""
}

// extractBody walks a message's MIME parts looking for a plain-text body,
// falling back to HTML if no plain-text part is found.
func extractBody(payload *gmail.MessagePart) string {
	if payload == nil {
		return ""
	}

	if body, ok := findPart(payload, "text/plain"); ok {
		return body
	}
	if body, ok := findPart(payload, "text/html"); ok {
		return body
	}
	return ""
}

func findPart(part *gmail.MessagePart, mimeType string) (string, bool) {
	if part.MimeType == mimeType && part.Body != nil && part.Body.Data != "" {
		return decodeBase64URL(part.Body.Data), true
	}
	for _, child := range part.Parts {
		if body, ok := findPart(child, mimeType); ok {
			return body, true
		}
	}
	return "", false
}

func decodeBase64URL(data string) string {
	decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(data)
	if err != nil {
		// Some payloads include padding; retry with standard padded decoding.
		decoded, err = base64.URLEncoding.DecodeString(data)
		if err != nil {
			return ""
		}
	}
	return string(decoded)
}
