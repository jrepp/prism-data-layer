package main

import (
	"flag"
	"log"
	"os"

	"github.com/jrepp/prism-data-layer/patterns/core"
	"github.com/jrepp/prism-data-layer/patterns/memstore"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Create MemStore plugin
	plugin := memstore.New()

	// Bootstrap plugin lifecycle
	if err := core.Bootstrap(plugin, *configPath); err != nil {
		log.Printf("Fatal error: %v\n", err)
		os.Exit(1)
	}
}
