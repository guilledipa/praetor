.PHONY: all proto build test clean

# The 'all' target compiles protos, runs tests, and builds binaries for everything
all: proto test build

proto:
	@echo "Compiling Protobufs..."
	protoc --go_out=./proto/gen/master --go-grpc_out=./proto/gen/master proto/master.proto \
		--go_opt=module=github.com/guilledipa/praetor/proto/gen/master \
		--go-grpc_opt=module=github.com/guilledipa/praetor/proto/gen/master
	@echo "Protobufs compiled!"

tidy:
	@echo "Tidying all workspace modules..."
	@cd proto && go mod tidy
	@cd proto/gen/master && go mod tidy
	@cd pkg && go mod tidy
	@cd agent && go mod tidy
	@cd master && go mod tidy
	@echo "Workspace tidied!"

test:
	@echo "Running test suite globally across the workspace..."
	@cd pkg && go test -v ./...
	@cd master && go test -v ./...
	@cd agent && go test -v ./...

build:
	@echo "Building Agent binary..."
	@cd agent && go build -o ../bin/agent ./cmd/agent
	@echo "Building Master binary..."
	@cd master && go build -o ../bin/master ./cmd/master
	@echo "Building praetorctl binary..."
	@cd cli && go build -o ../bin/praetorctl ./cmd/praetorctl
	@echo "Binaries exported to ./bin/"

clean:
	@echo "Cleaning binaries..."
	@rm -rf bin/
	@echo "Clean complete."
