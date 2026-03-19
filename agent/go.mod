module github.com/guilledipa/praetor/agent

go 1.22

replace github.com/guilledipa/praetor/proto/gen => ../proto/gen

replace github.com/guilledipa/praetor/agent/resources => ./resources

replace github.com/guilledipa/praetor/schema => ../schema

require (
	github.com/guilledipa/praetor/agent/resources v0.0.0-INCOMPATIBLE
	github.com/guilledipa/praetor/proto/gen v0.0.0-INCOMPATIBLE
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/nats-io/nats.go v1.34.0
	github.com/shirou/gopsutil/v3 v3.24.5
	google.golang.org/grpc v1.65.0
)

require (
	github.com/gabriel-vasile/mimetype v1.4.3 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.22.0 // indirect
	github.com/guilledipa/praetor/schema v0.0.0-INCOMPATIBLE // indirect
	github.com/klauspost/compress v1.17.2 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/crypto v0.23.0 // indirect
	golang.org/x/net v0.25.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	golang.org/x/text v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240528184218-531527333157 // indirect
	google.golang.org/protobuf v1.34.1 // indirect
)
