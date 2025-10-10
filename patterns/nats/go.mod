module github.com/jrepp/prism-data-layer/patterns/nats

go 1.24.0

require (
	github.com/jrepp/prism-data-layer/patterns/core v0.0.0
	github.com/nats-io/nats-server/v2 v2.12.0
	github.com/nats-io/nats.go v1.45.0
)

require (
	github.com/antithesishq/antithesis-sdk-go v0.4.3-default-no-op // indirect
	github.com/google/go-tpm v0.9.5 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/nats-io/jwt/v2 v2.8.0 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.42.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/text v0.29.0 // indirect
	golang.org/x/time v0.13.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
	google.golang.org/grpc v1.68.1 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/jrepp/prism-data-layer/patterns/core => ../core
