module github.com/jrepp/prism-data-layer/tests/acceptance/pattern-runner

go 1.24.0

require (
	github.com/jrepp/prism-data-layer/pkg/plugin v0.0.0
	github.com/jrepp/prism-data-layer/tests/interface-suites v0.0.0
	github.com/stretchr/testify v1.11.1
)

replace (
	github.com/jrepp/prism-data-layer/pkg/plugin => ../../../pkg/plugin
	github.com/jrepp/prism-data-layer/tests/interface-suites => ../../interface-suites
)
