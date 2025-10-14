module github.com/jrepp/prism-data-layer/tests/acceptance/cmd/compliance-report

go 1.24.0

require github.com/jrepp/prism-data-layer/tests/acceptance v0.0.0

require (
	github.com/jrepp/prism-data-layer/pkg/plugin v0.0.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/text v0.29.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250825161204-c5933d9347a5 // indirect
	google.golang.org/grpc v1.76.0 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
)

replace (
	github.com/jrepp/prism-data-layer/patterns/consumer => ../../../../patterns/consumer
	github.com/jrepp/prism-data-layer/patterns/producer => ../../../../patterns/producer
	github.com/jrepp/prism-data-layer/pkg/drivers/kafka => ../../../../pkg/drivers/kafka
	github.com/jrepp/prism-data-layer/pkg/drivers/memstore => ../../../../pkg/drivers/memstore
	github.com/jrepp/prism-data-layer/pkg/drivers/nats => ../../../../pkg/drivers/nats
	github.com/jrepp/prism-data-layer/pkg/drivers/postgres => ../../../../pkg/drivers/postgres
	github.com/jrepp/prism-data-layer/pkg/drivers/redis => ../../../../pkg/drivers/redis
	github.com/jrepp/prism-data-layer/pkg/drivers/s3 => ../../../../pkg/drivers/s3
	github.com/jrepp/prism-data-layer/pkg/patterns/common => ../../../../pkg/patterns/common
	github.com/jrepp/prism-data-layer/pkg/plugin => ../../../../pkg/plugin
	github.com/jrepp/prism-data-layer/tests/acceptance => ../..
	github.com/jrepp/prism-data-layer/tests/testing => ../../../testing
)
