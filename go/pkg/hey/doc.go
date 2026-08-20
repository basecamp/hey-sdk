// Package hey provides a Go SDK for the HEY API.
//
// The SDK handles authentication, HTTP caching, rate limiting, and retry logic.
// It supports both OAuth 2.0 authentication and static token authentication.
//
// # Installation
//
// To install the SDK, use go get:
//
//	go get github.com/basecamp/hey-sdk/go/pkg/hey
//
// # Authentication
//
// The SDK supports two authentication methods:
//
// Static Token Authentication (simplest):
//
//	cfg := hey.DefaultConfig()
//	token := &hey.StaticTokenProvider{Token: os.Getenv("HEY_TOKEN")}
//	client := hey.NewClient(cfg, token)
//
// OAuth 2.0 Authentication (for user-facing apps):
//
//	cfg := hey.DefaultConfig()
//	authMgr := hey.NewAuthManager(cfg, http.DefaultClient)
//	client := hey.NewClient(cfg, authMgr)
//
// # Services
//
// The SDK provides typed services for each HEY resource:
//
//   - [Client.Identity] - Current user identity and navigation
//   - [Client.Boxes] - Mailboxes (the Imbox, The Feed, Paper Trail, ...) and box groups
//   - [Client.Postings] - Bulk posting actions: seen, move, trash, spam, mute, file, bubble up
//   - [Client.Topics] - Topics and views (sent, spam, trash, everything), status and moves
//   - [Client.Messages] - Individual messages
//   - [Client.Attachments] - Active Storage direct uploads for outgoing attachments
//   - [Client.Entries] - Drafts, replies and forwards
//   - [Client.Contacts] - Contacts, notes, screening and bundling
//   - [Client.Calendars] - Calendar views and recordings
//   - [Client.CalendarTodos], [Client.CalendarEvents], [Client.Habits], [Client.TimeTracks], [Client.Journal] - What a calendar records
//   - [Client.Search] - Search
//   - [Client.Folders], [Client.Collections], [Client.Stickies], [Client.Clips], [Client.Snippets], [Client.Workflows] - Filing mail, and text kept to reuse
//   - [Client.Publications] - Public links for threads
//   - [Client.Designations], [Client.Extenzions], [Client.World] - Where mail lands, extra addresses, and HEY World
//
// # Linked Accounts and Separate Identities
//
// A root client represents one authenticated HEY identity and presents mail
// from All Accounts. [Client.ForAccount] derives an immutable client that
// presents mail and resolves acting senders and users for one linked account:
//
//	work, err := client.ForAccount(ctx, workAccountID)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	postings, err := work.Boxes().GetImbox(ctx, nil)
//
// Separate identities use separate root clients with their own token providers
// or authentication strategies. Each root client can derive its own linked
// account clients.
//
// Account scope follows HEY's mail-filter semantics and is not an authorization
// boundary. Identity-owned services such as Calendar and Journal remain
// identity-wide.
//
// # Working with Boxes
//
// List all mailboxes:
//
//	boxes, err := client.Boxes().List(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for _, b := range boxes {
//	    fmt.Println(b.Name)
//	}
//
// # Pagination
//
// The SDK handles pagination automatically via FollowPagination:
//
//	resp, err := client.Contacts().List(ctx, nil)
//	// The SDK follows Link headers for pagination
//
// # Error Handling
//
// The SDK returns typed errors that can be inspected:
//
//	_, err := client.Boxes().Get(ctx, 999)
//	if err != nil {
//	    var apiErr *hey.Error
//	    if errors.As(err, &apiErr) {
//	        switch apiErr.Code {
//	        case hey.CodeNotFound:
//	            // Handle 404
//	        case hey.CodeAuth:
//	            // Handle authentication error
//	        case hey.CodeRateLimit:
//	            // Handle rate limiting (auto-retried by default)
//	        }
//	    }
//	}
//
// # Thread Safety
//
// The Client is safe for concurrent use after construction.
// Service accessors (e.g., client.Boxes()) use mutex-protected lazy initialization.
package hey
