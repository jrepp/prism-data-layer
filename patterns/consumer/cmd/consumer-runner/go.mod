module github.com/jrepp/prism-data-layer/patterns/consumer/cmd/consumer-runner

go 1.24.0

require (
	github.com/jrepp/prism-data-layer/patterns/consumer v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/memstore v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/nats v0.0.0
	github.com/jrepp/prism-data-layer/pkg/plugin v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jrepp/prism-data-layer/pkg/patterns/common v0.0.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/nats-io/nats.go v1.45.0 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
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
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
	google.golang.org/grpc v1.76.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace (
	github.com/jrepp/prism-data-layer/patterns/consumer => ../..
	github.com/jrepp/prism-data-layer/pkg/drivers/memstore => ../../../../pkg/drivers/memstore
	github.com/jrepp/prism-data-layer/pkg/drivers/nats => ../../../../pkg/drivers/nats
	github.com/jrepp/prism-data-layer/pkg/patterns/common => ../../../../pkg/patterns/common
	github.com/jrepp/prism-data-layer/pkg/plugin => ../../../../pkg/plugin
)
