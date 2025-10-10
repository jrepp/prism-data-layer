module github.com/jrepp/prism-data-layer/patterns/memstore

go 1.24.0

require github.com/jrepp/prism-data-layer/patterns/core v0.0.0

require (
	golang.org/x/net v0.42.0 // indirect
	golang.org/x/sys v0.34.0 // indirect
	golang.org/x/text v0.27.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
	google.golang.org/grpc v1.76.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/jrepp/prism-data-layer/patterns/core => ../core
