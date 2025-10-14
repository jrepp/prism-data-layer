module github.com/jrepp/prism-data-layer/patterns/multicast_registry/cmd/multicast-registry-runner

go 1.25.1

require (
	github.com/jrepp/prism-data-layer/patterns/multicast_registry v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/kafka v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/nats v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/postgres v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/redis v0.0.0
	github.com/jrepp/prism-data-layer/pkg/plugin v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/confluentinc/confluent-kafka-go/v2 v2.6.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.7.3 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
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
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/text v0.29.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
	google.golang.org/grpc v1.76.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace github.com/jrepp/prism-data-layer/patterns/multicast_registry => ../..

replace github.com/jrepp/prism-data-layer/pkg/drivers/kafka => ../../../../pkg/drivers/kafka

replace github.com/jrepp/prism-data-layer/pkg/drivers/nats => ../../../../pkg/drivers/nats

replace github.com/jrepp/prism-data-layer/pkg/drivers/postgres => ../../../../pkg/drivers/postgres

replace github.com/jrepp/prism-data-layer/pkg/drivers/redis => ../../../../pkg/drivers/redis

replace github.com/jrepp/prism-data-layer/pkg/plugin => ../../../../pkg/plugin
