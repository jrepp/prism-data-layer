module github.com/jrepp/prism-data-layer/plugins/redis

go 1.23

require (
	github.com/jrepp/prism-data-layer/plugins/core v0.0.0
	github.com/redis/go-redis/v9 v9.7.0
	google.golang.org/grpc v1.76.0
)

replace github.com/jrepp/prism-data-layer/plugins/core => ../core
