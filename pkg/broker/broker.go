package broker

import (
	"context"
	"crypto/tls"
)

// Message represents a generic message received from the broker.
type Message interface {
	Subject() string
	Data() []byte
	Headers() map[string][]string
	Ack() error
}

// Subscription represents an active pull subscription.
type Subscription interface {
	Fetch(batch int) ([]Message, error)
}

// Broker defines the interface for underlying message brokers (NATS, Kafka, etc)
type Broker interface {
	Connect(name, url string, tlsConfig *tls.Config) error
	Close() error

	// Core Pub/Sub
	Publish(ctx context.Context, subject string, data []byte) error
	Subscribe(subject string, handler func(msg Message)) error

// Persistent Streams (like JetStream or Kafka topics)
EnsureStream(streamName string, subjects []string) error
PullSubscribe(subject, streamName, consumerName string) (Subscription, error)
}
