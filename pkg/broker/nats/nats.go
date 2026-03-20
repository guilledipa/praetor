package nats

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/guilledipa/praetor/pkg/broker"
	natsgo "github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type natsBroker struct {
	nc *natsgo.Conn
	js natsgo.JetStreamContext
}

type natsMessage struct {
	msg *natsgo.Msg
}

func (m *natsMessage) Subject() string { return m.msg.Subject }
func (m *natsMessage) Data() []byte    { return m.msg.Data }
func (m *natsMessage) Ack() error      { return m.msg.Ack() }
func (m *natsMessage) Headers() map[string][]string {
	res := make(map[string][]string)
	for k, v := range m.msg.Header {
		res[k] = v
	}
	return res
}

type natsSubscription struct {
	sub *natsgo.Subscription
}

func (s *natsSubscription) Fetch(batch int) ([]broker.Message, error) {
	msgs, err := s.sub.Fetch(batch, natsgo.MaxWait(10*time.Second))
	if err != nil {
		return nil, err
	}
	var res []broker.Message
	for _, m := range msgs {
		res = append(res, &natsMessage{msg: m})
	}
	return res, nil
}

// NewBroker returns a new NATS broker instance.
func NewBroker() broker.Broker {
	return &natsBroker{}
}

func (b *natsBroker) Connect(name, url string, tlsConfig *tls.Config) error {
	opts := []natsgo.Option{
		natsgo.Name(name),
	}
	if tlsConfig != nil {
		opts = append(opts, natsgo.Secure(tlsConfig))
	}
	nc, err := natsgo.Connect(url, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	b.nc = nc

	js, err := nc.JetStream()
	if err == nil {
		b.js = js
	}
	return nil
}

func (b *natsBroker) Close() error {
	if b.nc != nil {
		b.nc.Close()
	}
	return nil
}

func (b *natsBroker) Publish(ctx context.Context, subject string, data []byte) error {
	msg := natsgo.NewMsg(subject)
	msg.Data = data
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(msg.Header))

	if b.js == nil {
		return b.nc.PublishMsg(msg)
	}
	_, err := b.js.PublishMsg(msg)
	return err
}

func (b *natsBroker) Subscribe(subject string, handler func(msg broker.Message)) error {
	_, err := b.nc.Subscribe(subject, func(m *natsgo.Msg) {
		handler(&natsMessage{msg: m})
	})
	return err
}

func (b *natsBroker) EnsureStream(streamName string, subjects []string) error {
	if b.js == nil {
		return fmt.Errorf("jetstream not available")
	}
	_, err := b.js.AddStream(&natsgo.StreamConfig{
		Name:     streamName,
		Subjects: subjects,
	})
	if err != nil && err != natsgo.ErrStreamNameAlreadyInUse {
		return err
	}
	return nil
}

func (b *natsBroker) PullSubscribe(subject, streamName, consumerName string) (broker.Subscription, error) {
	if b.js == nil {
		return nil, fmt.Errorf("jetstream not available")
	}
	sub, err := b.js.PullSubscribe(subject, consumerName, natsgo.BindStream(streamName))
	if err != nil {
		return nil, err
	}
	return &natsSubscription{sub: sub}, nil
}
