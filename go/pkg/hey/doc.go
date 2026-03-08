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
//   - [Client.Boxes] - Mailboxes (imbox, feedbox, etc.)
//   - [Client.Topics] - Email topics and views (sent, spam, trash, everything)
//   - [Client.Messages] - Individual messages
//   - [Client.Entries] - Drafts and replies
//   - [Client.Contacts] - Contact management
//   - [Client.Calendars] - Calendar views and recordings
//   - [Client.CalendarTodos] - Calendar todo items
//   - [Client.Habits] - Habit tracking
//   - [Client.TimeTracks] - Time tracking
//   - [Client.Journal] - Journal entries
//   - [Client.Search] - Full-text search
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
