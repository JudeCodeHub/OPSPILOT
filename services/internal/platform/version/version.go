// Package version holds build metadata that is injected at link time.
//
// The Dockerfile sets both values with:
//
//	go build -ldflags "-X github.com/git4jude/opspilot/services/internal/platform/version.Version=v1.2.3 \
//	                   -X github.com/git4jude/opspilot/services/internal/platform/version.Commit=abc1234"
//
// They must stay plain string variables (not constants) for -X to work.
package version

// Version is the release version of the binary (e.g. a git tag). "dev" for local builds.
var Version = "dev"

// Commit is the git commit the binary was built from. "dev" for local builds.
var Commit = "dev"
