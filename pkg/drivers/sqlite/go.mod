module github.com/jrepp/prism-data-layer/pkg/drivers/sqlite

go 1.23

require (
	github.com/jrepp/prism-data-layer/pkg/plugin v0.0.0
	modernc.org/sqlite v1.28.0
)

replace github.com/jrepp/prism-data-layer/pkg/plugin => ../../plugin
