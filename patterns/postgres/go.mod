module github.com/jrepp/prism-data-layer/patterns/postgres

go 1.23

require (
	github.com/jrepp/prism-data-layer/patterns/core v0.0.0
	github.com/jackc/pgx/v5 v5.7.3
	google.golang.org/grpc v1.76.0
)

replace github.com/jrepp/prism-data-layer/patterns/core => ../core
