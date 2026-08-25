// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build resilience

package resilience

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	mobycontainer "github.com/moby/moby/api/types/container"
	mobynetwork "github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	clockport "github.com/Jersyfi/hubtask/core/port/clock"
	envport "github.com/Jersyfi/hubtask/core/port/environment"
	healthport "github.com/Jersyfi/hubtask/core/port/health"
	clientport "github.com/Jersyfi/hubtask/core/port/httpclient"
	mailport "github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	storageport "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	healthadapter "github.com/Jersyfi/hubtask/infrastructure/health"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
	mailadapter "github.com/Jersyfi/hubtask/infrastructure/mail"
	"github.com/Jersyfi/hubtask/infrastructure/observability"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	res "github.com/Jersyfi/hubtask/infrastructure/resilience"
	storageadapter "github.com/Jersyfi/hubtask/infrastructure/storage"
)

// minioImage and mailpitImage are overridable the way the PostgreSQL image is, so the support
// matrix can vary them without a code change.
func minioImage() string {
	if image := os.Getenv("HUBTASK_TEST_MINIO_IMAGE"); image != "" {
		return image
	}
	return "minio/minio:latest"
}

func mailpitImage() string {
	if image := os.Getenv("HUBTASK_TEST_MAILPIT_IMAGE"); image != "" {
		return image
	}
	return "axllent/mailpit:latest"
}

// rt1WriteKind is the row the core write path writes: a queue entry scheduled far into the
// future, the same trick RT-3 uses - evidence, not work, and no tenant fixture needed.
const rt1WriteKind = queue.Kind("test.rt1.write")

// freeLoopbackPort asks the kernel for a port nobody is listening on.
func freeLoopbackPort(ctx context.Context, t *testing.T) int {
	t.Helper()
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

// fixedHostPorts binds container ports to chosen host ports, on loopback. Fixed rather than
// random, because the daemon re-allocates a random binding on restart - and an endpoint that
// moved during the outage would turn "recovery without a restart" into a test of
// reconfiguration. In production the endpoint is a stable name; the fixed port is that name
// here. The imports this needs are testcontainers' own dependencies, not new ones.
func fixedHostPorts(bindings map[string]int) func(*mobycontainer.HostConfig) {
	return func(hostConfig *mobycontainer.HostConfig) {
		ports := mobynetwork.PortMap{}
		for containerPort, hostPort := range bindings {
			ports[mobynetwork.MustParsePort(containerPort)] = []mobynetwork.PortBinding{{
				HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: strconv.Itoa(hostPort),
			}}
		}
		hostConfig.PortBindings = ports
	}
}

// startDependency runs one container and registers its termination.
func startDependency(t *testing.T, request testcontainers.ContainerRequest) testcontainers.Container {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: request,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("starting %s: %v", request.Image, err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	return container
}

// stopContainer is the outage: a docker stop, so the adapters see a refused connection on a port
// that was alive a moment ago - what a dead MinIO or mail server actually looks like, rather than
// a handler scripted to answer 500.
func stopContainer(t *testing.T, container testcontainers.Container) {
	t.Helper()
	grace := 5 * time.Second
	if err := container.Stop(context.Background(), &grace); err != nil {
		t.Fatalf("stopping the container: %v", err)
	}
}

// startAgain brings the dependency back. Start re-runs the container's wait strategy, so when it
// returns the dependency is serving - the recovery the test then waits for is the breaker's alone.
func startAgain(t *testing.T, container testcontainers.Container) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := container.Start(ctx); err != nil {
		t.Fatalf("restarting the container: %v", err)
	}
}

// TestRT1AStoppedContainerDegradesExactlyItsOwnFeature is the container-backed sibling the
// stand-in's header promised (C-12): the same composition - adapter, breaker, bulkhead, probe,
// registry, metrics - wired the way cmd/server/main.go wires it, against a real MinIO and a real
// Mailpit that are stopped mid-flight, one at a time.
//
// One test rather than one per dependency, deliberately: "a stopped dependency degrades exactly
// its own feature and nothing else" is a claim about the whole report, and it can only be asserted
// while the other dependency is up, probed, and expected to stay clean. The rows under test are
// the object storage and SMTP rows of observability-reliability.md §7. The write path is real as
// well - rows through infrastructure/postgres into the suite's PostgreSQL - because "no optional
// dependency sits between a user and their data" is best proven against the real data path.
func TestRT1AStoppedContainerDegradesExactlyItsOwnFeature(t *testing.T) {
	ctx := context.Background()

	// --- The dependencies, on addresses that survive a restart -----------------------------
	minioPort := freeLoopbackPort(ctx, t)
	minio := startDependency(t, testcontainers.ContainerRequest{
		Image: minioImage(),
		Env: map[string]string{
			"MINIO_ROOT_USER":     "rt1",
			"MINIO_ROOT_PASSWORD": "rt1-not-a-secret",
		},
		Cmd:                []string{"server", "/data"},
		ExposedPorts:       []string{"9000/tcp"},
		HostConfigModifier: fixedHostPorts(map[string]int{"9000/tcp": minioPort}),
		WaitingFor: wait.ForHTTP("/minio/health/ready").
			WithPort("9000/tcp").WithStartupTimeout(2 * time.Minute),
	})

	smtpPort := freeLoopbackPort(ctx, t)
	mailAPIPort := freeLoopbackPort(ctx, t)
	mailpit := startDependency(t, testcontainers.ContainerRequest{
		Image:              mailpitImage(),
		ExposedPorts:       []string{"1025/tcp", "8025/tcp"},
		HostConfigModifier: fixedHostPorts(map[string]int{"1025/tcp": smtpPort, "8025/tcp": mailAPIPort}),
		// A database file rather than Mailpit's in-memory default: the container's filesystem
		// survives a stop, its memory does not, and the test wants to tell "the outage lost
		// mail" apart from "the mailbox forgot".
		Env: map[string]string{"MP_DATABASE": "/tmp/mailpit.db"},
		// Both halves, because the test needs both: the SMTP port to deliver, and the API to
		// prove delivery happened.
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("1025/tcp"),
			wait.ForHTTP("/api/v1/messages").WithPort("8025/tcp"),
		).WithDeadline(2 * time.Minute),
	})

	// --- The composition, as main.go builds it ---------------------------------------------
	c := newClock()

	metrics, err := observability.NewMetrics(envport.Config{Version: "test", Commit: "test"})
	if err != nil {
		t.Fatalf("building the metrics: %v", err)
	}
	defer func() { _ = metrics.Shutdown(context.Background()) }()

	onStateChange := func(dependency string, state res.BreakerState) {
		metrics.CircuitBreakerState(context.Background(), dependency, state.Level())
	}
	// Threshold 2 and one probe to close, like the stand-in: the thresholds are configuration,
	// and what is under test is the composition, not the numbers.
	storageBreaker := res.NewBreaker(res.BreakerConfig{
		Dependency: "object_storage", FailureThreshold: 2, SuccessThreshold: 1,
		OpenFor: 30 * time.Second, Now: c.Now, OnStateChange: onStateChange,
	})
	mailBreaker := res.NewBreaker(res.BreakerConfig{
		Dependency: mailadapter.Dependency, FailureThreshold: 2, SuccessThreshold: 1,
		OpenFor: 30 * time.Second, Now: c.Now, OnStateChange: onStateChange,
	})

	s3, err := storageadapter.NewS3Storage(envport.StorageConfig{
		Kind:     envport.StorageS3,
		Endpoint: fmt.Sprintf("http://127.0.0.1:%d", minioPort),
		// us-east-1 for the same reason as the conformance suite: CreateBucket sends no
		// location constraint, and this is the region for which none is needed.
		Region:       "us-east-1",
		Bucket:       "hubtask-media",
		AccessKey:    secret.New("rt1"),
		SecretKey:    secret.New("rt1-not-a-secret"),
		UsePathStyle: true,
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("building the storage adapter: %v", err)
	}
	if err := s3.CreateBucket(ctx); err != nil {
		t.Fatalf("creating the bucket: %v", err)
	}
	store := storageadapter.NewResilientStore(s3, storageBreaker,
		res.NewBulkhead(res.BulkheadConfig{Name: "s3", Capacity: 8}))

	sender := mailadapter.NewResilientSender(mailadapter.NewSMTP(envport.MailConfig{
		Host:     "127.0.0.1",
		Port:     smtpPort,
		From:     "hubtask@rt1.test",
		Security: envport.MailSecurityNone,
		Timeout:  5 * time.Second,
	}), mailBreaker, res.NewBulkhead(res.BulkheadConfig{Name: mailadapter.Dependency, Capacity: 4}))

	registry := healthadapter.NewRegistry("test", []string{"api"})
	registry.Register(storageadapter.NewProbe(storageBreaker))
	registry.Register(mailadapter.NewProbe(mailBreaker, true))
	registry.SetSignals(metrics)
	registry.MarkStarted()

	// --- The real write path ---------------------------------------------------------------
	unitOfWork := openUnitOfWork(ctx, t, testDSN(t))
	jobs := postgres.NewQueue(clockadapter.NewUUIDv7(clockadapter.System{}), clockadapter.System{})
	writeTask := func() error {
		return unitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
			// Far enough out that no worker claims it: the row is evidence, not work.
			return jobs.Enqueue(ctx, queue.Request{Kind: rt1WriteKind, RunAt: time.Now().Add(24 * time.Hour)})
		})
	}
	tasksWritten := func() int {
		return depthAt(ctx, t, unitOfWork, rt1WriteKind, clockport.Fixed(time.Now().Add(48*time.Hour)))
	}

	storeMedia := func(key string) error {
		content := []byte("rt1 evidence object")
		return store.Put(ctx, storageport.Upload{
			Key: key, Content: bytes.NewReader(content),
			Size: int64(len(content)), ContentType: "text/plain",
		})
	}
	sendMail := func() error {
		return sender.Send(ctx, mailport.Message{
			To: "operator@rt1.test", Subject: "RT-1", Body: "the dependency under test says hello",
		})
	}

	// --- Everything up ---------------------------------------------------------------------
	if err := storeMedia("media/before-the-outage"); err != nil {
		t.Fatalf("the media path failed while everything was up: %v", err)
	}
	if err := sendMail(); err != nil {
		t.Fatalf("the mail path failed while everything was up: %v", err)
	}
	waitForDelivered(t, mailAPIPort, 1)
	if report := registry.Report(ctx); report.Status != healthport.StatusOK {
		t.Fatalf("status = %s while everything is up, want ok", report.Status)
	}

	// --- The object storage goes down ------------------------------------------------------
	stopContainer(t, minio)
	for i := range 3 {
		if err := storeMedia(fmt.Sprintf("media/during-the-outage-%d", i)); err == nil {
			t.Error("the media path succeeded although the object storage is stopped")
		}
	}
	if state := storageBreaker.State(); state != res.BreakerOpen {
		t.Fatalf("storage breaker = %s after repeated failures, want open", state)
	}

	// The core write path stays open, and it stays fast: an open breaker answers without
	// dialling, so nothing about the outage reaches a user creating a task. The bound is what
	// convicts a wrongly-coupled write path - a single write routed through the outage would
	// spend the 5 s storage timeout on its own.
	start := time.Now()
	for range 20 {
		if err := writeTask(); err != nil {
			t.Fatalf("the write path was blocked by the outage: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("20 writes took %v during the outage - the write path is waiting on something", elapsed)
	}
	if written := tasksWritten(); written != 20 {
		t.Errorf("writes = %d, want 20", written)
	}

	// Exactly its own feature and nothing else: mail still delivers while the bucket is gone.
	if err := sendMail(); err != nil {
		t.Errorf("the mail path failed during the storage outage: %v", err)
	}
	waitForDelivered(t, mailAPIPort, 2)

	report := registry.Report(ctx)
	if report.Status != healthport.StatusDegraded {
		t.Errorf("status = %s during the outage, want degraded", report.Status)
	}
	assertDegradedExactly(t, report, mediaFeature)
	if ready, reason := registry.Ready(ctx); !ready {
		t.Errorf("the process reported itself unready over an optional dependency: %s", reason)
	}

	body := scrape(t, metrics)
	for _, want := range []string{
		`hubtask_circuit_breaker_state{dependency="object_storage"} 2`,
		`hubtask_dependency_up{dependency="object_storage"} 0`,
		`hubtask_dependency_up{dependency="smtp"} 1`,
		`hubtask_degraded_mode{feature="media"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the storage outage is missing from the metrics: %s", want)
		}
	}

	// --- The object storage returns --------------------------------------------------------
	startAgain(t, minio)
	waitForRecovery(t, c, "object_storage", func() error {
		return storeMedia("media/after-the-recovery")
	})
	if state := storageBreaker.State(); state != res.BreakerClosed {
		t.Fatalf("storage breaker = %s after recovery, want closed", state)
	}
	// Nothing was lost to the outage: the object stored before it is still there. A container
	// stop is not a wipe, and the adapter found its way back without being rebuilt.
	object, err := store.Get(ctx, "media/before-the-outage")
	if err != nil {
		t.Fatalf("the pre-outage object is gone after recovery: %v", err)
	}
	_ = object.Content.Close()

	report = registry.Report(ctx)
	if report.Status != healthport.StatusOK {
		t.Errorf("status = %s after recovery, want ok", report.Status)
	}
	if len(report.DegradedFeatures) != 0 {
		t.Errorf("degraded features = %v after recovery, want none", report.DegradedFeatures)
	}

	// --- The mail server goes down ---------------------------------------------------------
	stopContainer(t, mailpit)
	for range 3 {
		if err := sendMail(); err == nil {
			t.Error("the mail path succeeded although the mail server is stopped")
		}
	}
	if state := mailBreaker.State(); state != res.BreakerOpen {
		t.Fatalf("mail breaker = %s after repeated failures, want open", state)
	}

	// The same two claims from the other side: the write path is untouched, and the outage
	// belongs to notifications alone - media works while the mail server is gone.
	for range 20 {
		if err := writeTask(); err != nil {
			t.Fatalf("the write path was blocked by the mail outage: %v", err)
		}
	}
	if written := tasksWritten(); written != 40 {
		t.Errorf("writes = %d after both outages, want 40", written)
	}
	if err := storeMedia("media/during-the-mail-outage"); err != nil {
		t.Errorf("the media path failed during the mail outage: %v", err)
	}

	report = registry.Report(ctx)
	if report.Status != healthport.StatusDegraded {
		t.Errorf("status = %s during the mail outage, want degraded", report.Status)
	}
	assertDegradedExactly(t, report, mailadapter.NotificationsFeature)
	if ready, reason := registry.Ready(ctx); !ready {
		t.Errorf("the process reported itself unready over an optional dependency: %s", reason)
	}

	body = scrape(t, metrics)
	for _, want := range []string{
		`hubtask_circuit_breaker_state{dependency="smtp"} 2`,
		`hubtask_dependency_up{dependency="smtp"} 0`,
		`hubtask_dependency_up{dependency="object_storage"} 1`,
		`hubtask_degraded_mode{feature="notifications"} 1`,
		`hubtask_degraded_mode{feature="media"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the mail outage is missing from the metrics: %s", want)
		}
	}

	// --- The mail server returns -----------------------------------------------------------
	startAgain(t, mailpit)
	waitForRecovery(t, c, "smtp", sendMail)
	if state := mailBreaker.State(); state != res.BreakerClosed {
		t.Fatalf("mail breaker = %s after recovery, want closed", state)
	}
	// The recovery probe was a real message, and it arrived: Mailpit keeps its mailbox across a
	// stop, so the count carries on from before the outage.
	waitForDelivered(t, mailAPIPort, 3)

	report = registry.Report(ctx)
	if report.Status != healthport.StatusOK {
		t.Errorf("status = %s after both recoveries, want ok", report.Status)
	}
	if len(report.DegradedFeatures) != 0 {
		t.Errorf("degraded features = %v after both recoveries, want none", report.DegradedFeatures)
	}

	// A gauge that only ever goes up keeps showing an outage that ended hours ago.
	body = scrape(t, metrics)
	for _, want := range []string{
		`hubtask_circuit_breaker_state{dependency="object_storage"} 0`,
		`hubtask_circuit_breaker_state{dependency="smtp"} 0`,
		`hubtask_dependency_up{dependency="object_storage"} 1`,
		`hubtask_dependency_up{dependency="smtp"} 1`,
		`hubtask_degraded_mode{feature="media"} 0`,
		`hubtask_degraded_mode{feature="notifications"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the recovery is missing from the metrics: %s", want)
		}
	}
}

// assertDegradedExactly holds the report against one row of the degradation table: the one
// feature, its reason code, and a timestamp an operator can subtract from now.
func assertDegradedExactly(t *testing.T, report healthport.Report, feature string) {
	t.Helper()
	if len(report.DegradedFeatures) != 1 {
		t.Errorf("degraded features = %v, want exactly %q", report.DegradedFeatures, feature)
		return
	}
	degraded := report.DegradedFeatures[0]
	if degraded.Feature != feature {
		t.Errorf("degraded feature = %q, want %q", degraded.Feature, feature)
	}
	if degraded.ReasonCode != "dependency.unavailable" {
		t.Errorf("reason = %q, want dependency.unavailable", degraded.ReasonCode)
	}
	if degraded.Since.IsZero() {
		t.Error("the degradation carries no timestamp")
	}
}

// waitForRecovery drives the breaker through its cool-down until the dependency answers again:
// advance past OpenFor, let one probe through, repeat while it fails. What must NOT appear in
// this loop is any reconfiguration - no new adapter, no new endpoint, no restart. The loop is
// only the passage of time, which is exactly what production recovery consists of.
func waitForRecovery(t *testing.T, c *clock, dependency string, attempt func() error) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for {
		c.advance(31 * time.Second)
		err := attempt()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s did not recover: %v", dependency, err)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// waitForDelivered polls Mailpit's API until the mailbox holds the expected number of messages.
// Polled rather than read once: acceptance on the socket and visibility in the API are two
// moments, and the gap between them is Mailpit's business, not this test's.
func waitForDelivered(t *testing.T, apiPort int, want int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var last int
	for {
		last = deliveredCount(t, apiPort)
		if last == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the mailbox holds %d messages, want %d", last, want)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// deliveredCount asks Mailpit how many messages it holds, through the guarded client - the same
// way out of the process every other test call takes (rule 6).
func deliveredCount(t *testing.T, apiPort int) int {
	t.Helper()
	cfg := envport.OutboundConfig{
		Timeout:              5 * time.Second,
		ConnectTimeout:       2 * time.Second,
		MaxResponseBytes:     1 << 20,
		MaxRedirects:         1,
		AllowPrivateNetworks: true, // Mailpit listens on loopback
	}
	client := httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg))
	resp, err := client.Do(context.Background(), clientport.Request{
		URL:         fmt.Sprintf("http://127.0.0.1:%d/api/v1/messages", apiPort),
		TargetClass: "test",
	})
	if err != nil {
		t.Fatalf("asking Mailpit for its mailbox: %v", err)
	}
	if resp.Status != 200 {
		t.Fatalf("Mailpit answered %d", resp.Status)
	}
	var mailbox struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(resp.Body, &mailbox); err != nil {
		t.Fatalf("reading Mailpit's answer: %v", err)
	}
	return mailbox.Total
}
