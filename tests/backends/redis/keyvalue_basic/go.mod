module github.com/jrepp/prism-data-layer/tests/backends/redis/keyvalue_basic

go 1.24.0

require (
	github.com/jrepp/prism-data-layer/pkg/drivers/redis v0.0.0
	github.com/jrepp/prism-data-layer/pkg/plugin v0.0.0
	github.com/jrepp/prism-data-layer/tests/interface-suites v0.0.0
	github.com/stretchr/testify v1.11.1
	github.com/testcontainers/testcontainers-go v0.33.0
)

replace (
	github.com/jrepp/prism-data-layer/pkg/drivers/redis => ../../../../pkg/drivers/redis
	github.com/jrepp/prism-data-layer/pkg/plugin => ../../../../pkg/plugin
	github.com/jrepp/prism-data-layer/tests/interface-suites => ../../../interface-suites
)
