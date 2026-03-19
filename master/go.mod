module github.com/guilledipa/praetor/master

go 1.23.0

toolchain go1.24.4

replace github.com/guilledipa/praetor/proto/gen => ../proto/gen

require (
	github.com/guilledipa/praetor/proto/gen v0.0.0-INCOMPATIBLE
	github.com/guilledipa/praetor/schema v0.0.0-00010101000000-000000000000
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/nats-io/nats.go v1.46.1
	google.golang.org/grpc v1.65.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.22.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.37.0 // indirect
	golang.org/x/net v0.25.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240528184218-531527333157 // indirect
	google.golang.org/protobuf v1.34.1 // indirect
)

replace github.com/guilledipa/praetor/master/catalog => ./catalog

replace github.com/guilledipa/praetor/schema => ../schema

replace github.com/guilledipa/praetor/pkg => ../pkg
