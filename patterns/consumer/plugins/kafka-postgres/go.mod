module github.com/jrepp/prism-data-layer/patterns/consumer/plugins/kafka-postgres

go 1.24.0

require (
	github.com/jrepp/prism-data-layer/patterns/consumer v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/kafka v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/postgres v0.0.0
	github.com/jrepp/prism-data-layer/pkg/plugin v0.0.0
)

replace (
	github.com/jrepp/prism-data-layer/patterns/consumer => ../..
	github.com/jrepp/prism-data-layer/pkg/drivers/kafka => ../../../../pkg/drivers/kafka
	github.com/jrepp/prism-data-layer/pkg/drivers/postgres => ../../../../pkg/drivers/postgres
	github.com/jrepp/prism-data-layer/pkg/plugin => ../../../../pkg/plugin
)
