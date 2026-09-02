// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package eventbus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// This file is the only place in the repository that imports the NATS client, and a gate in
// test/architecture says so (ADR-0041). Everything above it speaks the event bus port; a bus
// swapped tomorrow changes no event anybody publishes.

// typePrefix is the namespace every event type this system emits begins with
// (`de.hubtask.work.item.created.v1`). It is stripped from the subject because the configured
// prefix already namespaces the stream, and repeating it would make every subject four tokens
// longer for no routing anybody can use.
const typePrefix = "de.hubtask."

// PublisherConfig is what an operator's bus needs from this side.
type PublisherConfig struct {
	// URL is the server or the cluster, in the client's own comma-separated form
	// (`nats://host:4222`). Empty means no bus, and nothing here is built.
	URL string
	// SubjectPrefix is the first token of every subject. Its own setting because a stream is the
	// operator's, and binding one to `hubtask.>` is their configuration rather than ours.
	SubjectPrefix string
	// CredentialsFile is the path to a NATS credentials file (the nkeys/JWT form `nsc` writes),
	// or empty for a server that takes none.
	//
	// A path and not the contents, which is the same choice every other secret here makes: a
	// Docker or Kubernetes secret is a mounted file, the client reads it itself, and the JWT and
	// the seed inside it never become a string this process holds, logs or formats.
	CredentialsFile string
	// ConnectTimeout bounds the first connection. A bus that cannot be reached at startup must not
	// hold the process from serving: the connection is retried in the background and the publish
	// path reports it as unavailable meanwhile.
	ConnectTimeout time.Duration
	// PublishTimeout bounds one publish, ack included. No call without a deadline (ADR-0016).
	PublishTimeout time.Duration
}

// Publisher puts CloudEvents on a JetStream stream.
//
// The stream itself is the operator's to create and to bind to `<prefix>.>`. This does not create
// one, and that is a decision rather than an omission: a stream carries a retention policy, a
// replica count and a storage class, and picking those on somebody's behalf is picking how much
// of their disk this system may use.
type Publisher struct {
	config PublisherConfig

	mu     sync.RWMutex
	conn   *nats.Conn
	stream jetstream.JetStream
}

// NewPublisher connects, and does not fail the process when it cannot.
//
// A bus that is down at startup is the degraded state observability-reliability.md §7 describes,
// not a reason to refuse to serve: the outbox holds the events, the job retries, and the
// connection is re-established by the client's own reconnection loop. So a failed first connect is
// returned as a Publisher that reports itself unavailable, and never as a startup error.
func NewPublisher(ctx context.Context, config PublisherConfig) *Publisher {
	publisher := &Publisher{config: config}

	options := []nats.Option{
		nats.Name("hubtask"),
		nats.Timeout(config.ConnectTimeout),
		// Forever, with a bounded wait between attempts. The alternative is a client that gives
		// up while the outbox keeps filling, and an operator who has to restart a process to make
		// their bus work again after a maintenance window.
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
	}
	if config.CredentialsFile != "" {
		options = append(options, nats.UserCredentials(config.CredentialsFile))
	}

	conn, err := nats.Connect(config.URL, options...)
	if err != nil {
		// Deliberately not returned. The publish path answers ErrUnavailable until the connection
		// exists, which is the same answer it gives when the bus goes away later - one degraded
		// state rather than two.
		return publisher
	}

	stream, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return publisher
	}

	publisher.mu.Lock()
	publisher.conn, publisher.stream = conn, stream
	publisher.mu.Unlock()

	context.AfterFunc(ctx, publisher.Close)
	return publisher
}

// Publish puts one rendered CloudEvent on the subject its type and tenant name.
//
// It waits for the ack. A publish that returned before the server had the message would make
// "delivered" mean "handed to a socket", and the job would be marked done for an event nobody
// received - which is exactly the failure the outbox exists to prevent one layer down.
func (p *Publisher) Publish(ctx context.Context, tenantID shared.ID, eventType string, payload []byte) error {
	p.mu.RLock()
	stream := p.stream
	p.mu.RUnlock()

	if stream == nil {
		return busUnavailable(errors.New("no connection to the bus"))
	}

	publishCtx, cancel := context.WithTimeout(ctx, p.config.PublishTimeout)
	defer cancel()

	if _, err := stream.Publish(publishCtx, Subject(p.config.SubjectPrefix, tenantID, eventType), payload); err != nil {
		return busUnavailable(err)
	}
	return nil
}

// Connected reports whether the bus is reachable right now, which is what /meta/health asks.
func (p *Publisher) Connected() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.conn != nil && p.conn.IsConnected()
}

// Close gives the connection up, draining what is in flight.
func (p *Publisher) Close() {
	p.mu.Lock()
	conn := p.conn
	p.conn, p.stream = nil, nil
	p.mu.Unlock()

	if conn != nil {
		// Drain rather than Close: a publish that has been handed to the client and not yet
		// acknowledged is one this process still owes an answer for.
		_ = conn.Drain()
	}
}

// Subject is where an event lands: `<prefix>.<tenant>.<type without de.hubtask.>`.
//
// The tenant is a subject token and not only an attribute, and that is the NATS reading of what
// ADR-0007 already decided: the tenant travels where a broker can route on it, and in NATS the
// subject *is* where routing happens. A consumer wanting one workspace binds `<prefix>.<id>.>`
// instead of filtering every message it receives.
func Subject(prefix string, tenantID shared.ID, eventType string) string {
	return strings.Join([]string{
		prefix,
		tenantID.String(),
		strings.TrimPrefix(eventType, typePrefix),
	}, ".")
}

// busUnavailable is the one shape a bus failure takes on the way out: UNAVAILABLE, so the job
// retries on the queue's ladder and the breaker counts it as the dependency's fault rather than
// the caller's (ADR-0016).
func busUnavailable(cause error) error {
	return shared.ErrUnavailable.
		WithDetail("bus.unavailable").
		WithCause(fmt.Errorf("nats: %w", cause))
}
