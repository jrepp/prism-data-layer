module github.com/jrepp/prism-data-layer/tests/acceptance/pattern-runner

go 1.24.0

replace (
	github.com/jrepp/prism-data-layer/pkg/plugin => ../../../pkg/plugin
	github.com/jrepp/prism-data-layer/tests/interface-suites => ../../interface-suites
)
