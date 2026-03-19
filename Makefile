.PHONY: all proto build test clean

# The 'all' target compiles protos, runs tests, and builds binaries for everything
all: proto test build

proto:
	@echo "Compiling master protobuf..."
	@protoc --go_out=./proto/gen/master --go-grpc_out=./proto/gen/master proto/master.proto --go_opt=module=github.com/guilledipa/praetor/proto/gen/master --go-grpc_opt=module=github.com/guilledipa/praetor/proto/gen/master
	@echo "Compiling plugin protobuf..."
	@mkdir -p proto/gen/plugin
	@protoc --go_out=./proto/gen/plugin --go-grpc_out=./proto/gen/plugin proto/plugin.proto --go_opt=module=github.com/guilledipa/praetor/proto/gen/plugin --go-grpc_opt=module=github.com/guilledipa/praetor/proto/gen/plugin
	@echo "Tidying proto dependencies..."
	@cd proto/gen/master && go mod tidy
	@cd proto/gen/plugin && go mod init github.com/guilledipa/praetor/proto/gen/plugin || true
	@cd proto/gen/plugin && go mod tidy
	@echo "Protobufs compiled and tidied!"

tidy:
	@echo "Tidying all workspace modules..."
	@cd proto && go mod tidy
	@cd proto/gen/master && go mod tidy
	@cd proto/gen/plugin && go mod tidy
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
	@echo "Building Agent..."
	@cd agent && go build -o ../bin/praetor-agent cmd/agent/main.go
	@echo "Building Master..."
	@cd master && go build -o ../bin/praetor-master cmd/master/main.go
	@echo "Building praetor-plugin-linux..."
	@cd plugins/linux && go build -o ../../bin/praetor-plugin-linux cmd/praetor-plugin-linux/main.go
	@echo "Building praetorctl..."
	@cd cli && go build -o ../bin/praetorctl ./cmd/praetorctl
	@echo "Build successful! Binaries are in ./bin/"

clean:
	@echo "Cleaning binaries..."
	@rm -rf bin/
	@echo "Clean complete."
