module github.com/prism/cmd/prism-loadtest

go 1.21

require (
	github.com/prism/patterns/multicast_registry v0.0.0
	github.com/spf13/cobra v1.8.0
	golang.org/x/time v0.5.0
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.17.4 // indirect
	github.com/nats-io/nats.go v1.31.0 // indirect
	github.com/nats-io/nkeys v0.4.6 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/redis/go-redis/v9 v9.3.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/crypto v0.16.0 // indirect
	golang.org/x/sys v0.34.0 // indirect
)

replace github.com/prism/patterns/multicast_registry => ../../patterns/multicast_registry

replace github.com/prism/patterns/core => ../../patterns/core
