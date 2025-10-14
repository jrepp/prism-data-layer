module github.com/jrepp/prism-data-layer/patterns/consumer/plugins/nats-redis

go 1.24.0

require (
	github.com/jrepp/prism-data-layer/patterns/consumer v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/nats v0.0.0
	github.com/jrepp/prism-data-layer/pkg/drivers/redis v0.0.0
	github.com/jrepp/prism-data-layer/pkg/plugin v0.0.0
)

replace (
	github.com/jrepp/prism-data-layer/patterns/consumer => ../..
	github.com/jrepp/prism-data-layer/pkg/drivers/nats => ../../../../pkg/drivers/nats
	github.com/jrepp/prism-data-layer/pkg/drivers/redis => ../../../../pkg/drivers/redis
	github.com/jrepp/prism-data-layer/pkg/plugin => ../../../../pkg/plugin
)
