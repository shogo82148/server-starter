package starter

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// these variable is set by goreleaser
var version = "" // .Version

func showVersion() {
	fmt.Printf("Yet Another Go port of start_server by shogo82148 version %s %s/%s (built by %s)\n", getVersion(), runtime.GOOS, runtime.GOARCH, runtime.Version())
}

func getVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return info.Main.Version
}
