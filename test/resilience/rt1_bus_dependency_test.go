// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build resilience

package resilience

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	healthport "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/infrastructure/eventbus"
	res "github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// This file imports the NATS client, and it is the one place outside infrastructure/eventbus that
// may: the architecture gate ADR-0041 asks for covers core, infrastructure, presentation and cmd,
// and deliberately not test/. A test that proves the adapter delivers has to be able to look at
// the other end of the connection, and looking at it through the adapter would prove that the
// adapter agrees with itself.

const busSubjectPrefix = "hubtask"

func natsImage() string {
	if image := os.Getenv("HUBTASK_TEST_NATS_IMAGE"); image != "" {
		return image
	}
	return "nats:2-alpine"
}

// startBus brings up a JetStream server and a stream bound to the prefix this system publishes
// under. The stream is created here because it is the operator's act, and in this test the test is
// the operator - the adapter deliberately does not create one (ADR-0041).
func startBus(ctx context.Context, t *testing.T) (testcontainers.Container, string) {
	t.Helper()

	// A fixed loopback port, through the helper RT-1 already has and for the reason it already
	// states: the daemon re-allocates a random binding on restart, and an endpoint that moved
	// during the outage would turn "recovery without a restart" into a test of reconfiguration.
	port := freeLoopbackPort(ctx, t)
	return startBusOn(ctx, t, port), fmt.Sprintf("nats://127.0.0.1:%d", port)
}

// startBusOn is the half of it that a test which needs the address *before* the server exists
// calls on its own.
func startBusOn(ctx context.Context, t *testing.T, port int) testcontainers.Container {
	t.Helper()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:              natsImage(),
			Cmd:                []string{"-js", "-sd", "/tmp/jetstream"},
			ExposedPorts:       []string{"4222/tcp"},
			HostConfigModifier: fixedHostPorts(map[string]int{"4222/tcp": port}),
			WaitingFor:         wait.ForLog("Server is ready").WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("starting NATS: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })

	return container
}

// createStream binds a stream to `<prefix>.>`, which is what an operator does once.
func createStream(ctx context.Context, t *testing.T, url string) {
	t.Helper()
	conn, err := nats.Connect(url, nats.Timeout(10*time.Second))
	if err != nil {
		t.Fatalf("connecting to the bus: %v", err)
	}
	defer conn.Close()

	stream, err := jetstream.New(conn)
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	if _, err := stream.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     "HUBTASK",
		Subjects: []string{busSubjectPrefix + ".>"},
	}); err != nil {
		t.Fatalf("creating the stream: %v", err)
	}
}

// RT-1's discipline, applied to the dependency H-14 added: an optional dependency failing must not
// block the write path, /meta/health must say so, and recovery must happen without a restart
// (observability-reliability.md §7, §12).
//
// What is under test is the adapter and the breaker in front of it. That the outbox holds the
// events meanwhile is the dispatcher's own property, proved where the dispatcher is - here it is
// the premise: nothing this test does can lose an event, because nothing this test does removes
// one from the outbox.
func TestRT1TheBusFailingDegradesAndRecoversWithoutARestart(t *testing.T) {
	ctx := context.Background()
	container, url := startBus(ctx, t)
	createStream(ctx, t, url)

	breaker := res.NewBreaker(res.BreakerConfig{
		Dependency: eventbus.BusDependency, FailureThreshold: 2, SuccessThreshold: 1,
		OpenFor: 200 * time.Millisecond,
	})
	probe := eventbus.NewProbe(breaker, true)
	bus := eventbus.NewResilient(eventbus.NewPublisher(ctx, eventbus.PublisherConfig{
		URL:            url,
		SubjectPrefix:  busSubjectPrefix,
		ConnectTimeout: 10 * time.Second,
		PublishTimeout: 5 * time.Second,
	}), breaker)

	tenant := shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")
	const eventType = "de.hubtask.work.item.created.v1"

	// 1. It works, and the message is on the stream. Read back through a consumer rather than
	//    trusting the ack: an ack that the adapter believed and the server did not is exactly the
	//    failure the ack is supposed to rule out.
	if err := bus.Publish(ctx, tenant, eventType, []byte(`{"id":"first"}`)); err != nil {
		t.Fatalf("publishing to a healthy bus: %v", err)
	}
	if got := messagesOnStream(ctx, t, url); got != 1 {
		t.Fatalf("the stream holds %d messages after one publish", got)
	}
	if result := probe.Check(ctx); result.Status != healthport.StatusOK {
		t.Errorf("the probe reports %s while the bus is up", result.Status)
	}

	// 2. The bus goes away.
	if err := container.Stop(ctx, nil); err != nil {
		t.Fatalf("stopping NATS: %v", err)
	}

	// Kept publishing rather than stopping at the first refusal: the breaker opens on its
	// threshold, so one failure is a failure and not yet a state. The first version of this test
	// stopped at one and then asked the probe why it still said the bus was up.
	var refusal error
	for range 10 {
		if err := bus.Publish(ctx, tenant, eventType, []byte(`{"id":"during"}`)); err != nil {
			refusal = err
		}
		if probe.Check(ctx).Status == healthport.StatusDown {
			break
		}
	}
	if refusal == nil {
		t.Fatal("publishing to a stopped bus was reported as a success")
	}
	if category := shared.AsError(refusal).Category; category != shared.CategoryUnavailable {
		t.Errorf("the refusal is a %s, want unavailable - the queue retries on that and only that", category)
	}

	// 3. And /meta/health says so, with the impact named rather than left to be guessed.
	report := probe.Check(ctx)
	if report.Status != healthport.StatusDown {
		t.Errorf("the probe reports %s while the bus is stopped", report.Status)
	}
	if len(report.Impact) == 0 {
		t.Error("the report names no impact; a dependency reported down with no consequence reads as an omission")
	}

	// 4. It comes back, and delivery resumes without anything being restarted. The publisher is
	//    the same object throughout: the client's own reconnection is what recovers, which is what
	//    "without a restart" means for an operator whose maintenance window has ended.
	if err := container.Start(ctx); err != nil {
		t.Fatalf("starting NATS again: %v", err)
	}
	if !eventuallyPublishes(ctx, bus, tenant, eventType) {
		t.Fatal("the bus never accepted a publish again after it returned")
	}
	if result := probe.Check(ctx); result.Status != healthport.StatusOK {
		t.Errorf("the probe still reports %s after the bus returned", result.Status)
	}
}

// messagesOnStream asks the server how many messages it holds.
func messagesOnStream(ctx context.Context, t *testing.T, url string) uint64 {
	t.Helper()
	conn, err := nats.Connect(url, nats.Timeout(10*time.Second))
	if err != nil {
		t.Fatalf("connecting to the bus: %v", err)
	}
	defer conn.Close()

	stream, err := jetstream.New(conn)
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	found, err := stream.Stream(ctx, "HUBTASK")
	if err != nil {
		t.Fatalf("reading the stream: %v", err)
	}
	info, err := found.Info(ctx)
	if err != nil {
		t.Fatalf("stream info: %v", err)
	}
	return info.State.Msgs
}

// eventuallyPublishes retries until the bus takes a message or the deadline runs out.
//
// It does not classify what comes back, and deliberately: the refusal's shape is asserted above
// while the bus is down, and here every answer that is not a success is just "not yet" - the
// breaker's own open-circuit refusal among them, which is the state the recovery has to pass
// through rather than a finding.
func eventuallyPublishes(ctx context.Context, bus eventbus.Resilient, tenant shared.ID, eventType string) bool {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if err := bus.Publish(ctx, tenant, eventType, []byte(`{"id":"after"}`)); err == nil {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// A bus that is down while this process starts must be picked up when it appears, and that is not
// free: without `RetryOnFailedConnect` the client's Connect returns an error, there is no
// connection to reconnect, and the reconnection loop never runs - so the adapter would answer
// "unavailable" for ever while the bus sat there working.
//
// The adapter's own documentation made that promise before the option did. This is the test that
// keeps it.
func TestRT1ABusThatIsDownAtStartupIsPickedUpWhenItAppears(t *testing.T) {
	ctx := context.Background()
	port := freeLoopbackPort(ctx, t)
	url := fmt.Sprintf("nats://127.0.0.1:%d", port)

	// Built against an address where nothing is listening. Nothing about this may fail, block or
	// panic: a bus that is down at startup is a degraded state, not a startup error.
	breaker := res.NewBreaker(res.BreakerConfig{
		Dependency: eventbus.BusDependency, FailureThreshold: 2, SuccessThreshold: 1,
		OpenFor: 200 * time.Millisecond,
	})
	bus := eventbus.NewResilient(eventbus.NewPublisher(ctx, eventbus.PublisherConfig{
		URL:            url,
		SubjectPrefix:  busSubjectPrefix,
		ConnectTimeout: 2 * time.Second,
		PublishTimeout: 2 * time.Second,
	}), breaker)

	tenant := shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")
	const eventType = "de.hubtask.work.item.created.v1"

	if err := bus.Publish(ctx, tenant, eventType, []byte(`{"id":"before"}`)); err == nil {
		t.Fatal("publishing to an address nothing listens on was reported as a success")
	}

	// The bus arrives.
	startBusOn(ctx, t, port)
	createStream(ctx, t, url)

	if !eventuallyPublishes(ctx, bus, tenant, eventType) {
		t.Fatal("the publisher never reached a bus that appeared after it was built")
	}
}
