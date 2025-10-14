module github.com/jrepp/prism-data-layer/tests/backends/memstore/keyvalue_basic

go 1.24.0

require (
	github.com/jrepp/prism-data-layer/pkg/drivers/memstore v0.0.0
	github.com/jrepp/prism-data-layer/pkg/plugin v0.0.0
	github.com/jrepp/prism-data-layer/tests/interface-suites v0.0.0
	github.com/stretchr/testify v1.11.1
)

replace (
	github.com/jrepp/prism-data-layer/pkg/drivers/memstore => ../../../../pkg/drivers/memstore
	github.com/jrepp/prism-data-layer/pkg/plugin => ../../../../pkg/plugin
	github.com/jrepp/prism-data-layer/tests/interface-suites => ../../../interface-suites
)
