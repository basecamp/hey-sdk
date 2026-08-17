module github.com/basecamp/hey-sdk/conformance/runner/go

go 1.26

require github.com/basecamp/hey-sdk/go v0.0.0

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/oapi-codegen/runtime v1.2.0 // indirect
	github.com/zalando/go-keyring v0.2.8 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

replace github.com/basecamp/hey-sdk/go => ../../../go
