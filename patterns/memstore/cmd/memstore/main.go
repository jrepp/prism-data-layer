package main

import (
	"github.com/jrepp/prism-data-layer/patterns/core"
	"github.com/jrepp/prism-data-layer/patterns/memstore"
)

// main is the entrypoint for the memstore backend driver.
//
// All boilerplate (config loading, flag parsing, lifecycle management) is
// handled by core.ServeBackendDriver(). This keeps main.go minimal and moves
// common logic into the SDK.
//
// The backend driver (memstore.New()) should not connect to any external
// resources until Initialize() or Start() lifecycle methods are called.
func main() {
	core.ServeBackendDriver(func() core.Plugin {
		return memstore.New()
	}, core.ServeOptions{
		DefaultName:    "memstore",
		DefaultVersion: "0.1.0",
		DefaultPort:    0, // Use 0 for dynamic port allocation
		ConfigPath:     "config.yaml",
	})
}
