module github.com/guilledipa/praetor/plugins/linux

go 1.25.0

replace github.com/guilledipa/praetor/pkg => ../../pkg

replace github.com/guilledipa/praetor/schema => ../../schema

replace github.com/guilledipa/praetor/proto/gen/plugin => ../../proto/gen/plugin

require (
	github.com/guilledipa/praetor/agent v0.0.0-00010101000000-000000000000
	github.com/guilledipa/praetor/pkg v0.0.0-INCOMPATIBLE
	github.com/guilledipa/praetor/schema v0.0.0-INCOMPATIBLE
	github.com/hashicorp/go-plugin v1.7.0
)

require (
	github.com/fatih/color v1.18.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.22.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/guilledipa/praetor/proto/gen/plugin v0.0.0-INCOMPATIBLE // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.42.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/grpc v1.79.3 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/guilledipa/praetor/agent => ../../agent
