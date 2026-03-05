module github.com/basecamp/hey-sdk/conformance/runner/go

go 1.24

require github.com/basecamp/hey-sdk/go v0.0.0

require (
	al.essio.dev/pkg/shellescape v1.5.1 // indirect
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/danieljoos/wincred v1.2.2 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/oapi-codegen/runtime v1.2.0 // indirect
	github.com/zalando/go-keyring v0.2.6 // indirect
	golang.org/x/sys v0.30.0 // indirect
)

replace github.com/basecamp/hey-sdk/go => ../../../go
