FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy all the modules needed due to go.work
COPY go.work go.work.sum /app/
COPY proto/ /app/proto/
COPY schema/ /app/schema/
COPY pkg/ /app/pkg/
COPY master/ /app/master/
COPY agent/ /app/agent/
COPY cli/ /app/cli/
COPY plugins/ /app/plugins/

# Build the master
WORKDIR /app/master
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /praetor-master cmd/master/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /praetor-master /app/praetor-master

# Expose gRPC port
EXPOSE 50051

CMD ["/app/praetor-master"]
