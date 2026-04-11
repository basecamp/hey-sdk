# HEY SDK test

Official SDK for the HEY API. Multi-language support for Go, TypeScript, Ruby, Swift, and Kotlin.

## Quick Start

### Go

```go
import "github.com/basecamp/hey-sdk/go/pkg/hey"

client := hey.NewClient(&hey.Config{}, &hey.StaticTokenProvider{Token: "your-token"})
boxes, err := client.Boxes().List(ctx)
```

### TypeScript

```typescript
import { HEYClient } from '@basecamp/hey-sdk'

const client = new HEYClient({ token: 'your-token' })
const boxes = await client.boxes.list()
```

### Ruby

```ruby
require 'hey-sdk'

client = HEY::Client.new(token: 'your-token')
boxes = client.boxes.list
```

## Development

```bash
make check          # Run all checks
make {lang}-test    # Run tests for one language
make conformance    # Run cross-language conformance tests
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for full development workflow.
