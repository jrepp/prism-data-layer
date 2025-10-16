module github.com/jrepp/prism-data-layer/patterns/mailbox

go 1.23

require (
	github.com/jrepp/prism-data-layer/pkg/plugin v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/nats v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/sqlite v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/jrepp/prism-data-layer/pkg/plugin => ../../pkg/plugin
replace github.com/jrepp/prism-data-layer/pkg/drivers/nats => ../../pkg/drivers/nats
replace github.com/jrepp/prism-data-layer/pkg/drivers/sqlite => ../../pkg/drivers/sqlite
