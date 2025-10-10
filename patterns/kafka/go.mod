module github.com/jrepp/prism-data-layer/patterns/kafka

go 1.23

require (
	github.com/jrepp/prism-data-layer/patterns/core v0.0.0
	github.com/confluentinc/confluent-kafka-go/v2 v2.6.1
	google.golang.org/grpc v1.76.0
)

replace github.com/jrepp/prism-data-layer/patterns/core => ../core
