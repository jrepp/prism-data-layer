module github.com/jrepp/prism-data-layer/patterns/producer

go 1.24.0

require (
	github.com/google/uuid v1.6.0
	github.com/jrepp/prism-data-layer/pkg/drivers/memstore v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/nats v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/redis v0.0.0-20251014161646-c7322ff385ad
	github.com/jrepp/prism-data-layer/pkg/patterns/common v0.0.0
	github.com/jrepp/prism-data-layer/pkg/plugin v0.0.0
	github.com/nats-io/nats-server/v2 v2.12.0
)

require (
	github.com/antithesishq/antithesis-sdk-go v0.4.3-default-no-op // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/go-tpm v0.9.5 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/nats-io/jwt/v2 v2.8.0 // indirect
	github.com/nats-io/nats.go v1.45.0 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/redis/go-redis/v9 v9.14.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel v1.37.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.24.0 // indirect
	go.opentelemetry.io/otel/metric v1.37.0 // indirect
	go.opentelemetry.io/otel/sdk v1.37.0 // indirect
	go.opentelemetry.io/otel/trace v1.37.0 // indirect
	golang.org/x/crypto v0.42.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/text v0.29.0 // indirect
	golang.org/x/time v0.13.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
	google.golang.org/grpc v1.76.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/jrepp/prism-data-layer/pkg/drivers/memstore => ../../pkg/drivers/memstore
	github.com/jrepp/prism-data-layer/pkg/drivers/nats => ../../pkg/drivers/nats
	github.com/jrepp/prism-data-layer/pkg/drivers/redis => ../../pkg/drivers/redis
	github.com/jrepp/prism-data-layer/pkg/patterns/common => ../../pkg/patterns/common
	github.com/jrepp/prism-data-layer/pkg/plugin => ../../pkg/plugin
)
