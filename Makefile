.PHONY: all proto build test clean

# The 'all' target compiles protos, runs tests, and builds binaries for everything
all: proto test build

proto:
	@echo "Compiling Protobufs..."
	@cd proto && protoc --go_out=. --go-grpc_out=. master.proto
	@echo "Protobufs compiled!"

test:
	@echo "Running test suite globally across the workspace..."
	@go test -v ./...

build:
	@echo "Building Agent binary..."
	@cd agent && go build -o ../bin/agent .
	@echo "Building Master binary..."
	@cd master && go build -o ../bin/master .
	@echo "Binaries exported to ./bin/"

clean:
	@echo "Cleaning binaries..."
	@rm -rf bin/
	@echo "Clean complete."
