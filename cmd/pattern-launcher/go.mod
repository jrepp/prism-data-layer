module github.com/jrepp/prism/cmd/pattern-launcher

go 1.24.0

replace (
	github.com/jrepp/prism-data-layer/pkg/isolation => ../../pkg/isolation
	github.com/jrepp/prism-data-layer/pkg/launcher => ../../pkg/launcher
	github.com/jrepp/prism-data-layer/pkg/plugin => ../../pkg/plugin
	github.com/jrepp/prism-data-layer/pkg/procmgr => ../../pkg/procmgr
)

require (
	github.com/jrepp/prism-data-layer/pkg/isolation v0.0.0
	github.com/jrepp/prism-data-layer/pkg/launcher v0.0.0
	github.com/jrepp/prism-data-layer/pkg/plugin v0.0.0
	google.golang.org/grpc v1.68.1
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/jrepp/prism-data-layer/pkg/procmgr v0.0.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250804133106-a7a43d27e69b // indirect
	google.golang.org/protobuf v1.36.8 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
