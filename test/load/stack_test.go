// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build load

// Package load holds the runs that need capacity rather than a container: RT-6's overload, the
// automation storm, and the regression guard that compares a run against a stored baseline
// (observability-reliability.md §12, ci-cd.md §3).
//
// They are behind the `load` tag and out of the pull request path for the reason ci-cd.md states
// as a rule: a test that runs in minutes on a shared runner is a gate, a test that needs sustained
// load is nightly. The harness's own arithmetic is not - it lives untagged beside this and runs in
// gate-unit, because a guard that has quietly stopped working must redden a pull request rather
// than pass a nightly.
package load

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

const (
	appPassword = "test-only-not-a-secret"
	// installationSecret is what the token hashes are peppered with. A fixed value, because the
	// stack and the credentials it mints come up together and neither outlives the run.
	installationSecret = "load-test-installation-secret-not-a-real-one"
	// encryptionKey is 32 bytes, base64, and fixed: it is the key ring of an installation that
	// lives for two minutes.
	encryptionKey = "bG9hZC10ZXN0LWtleS0zMi1ieXRlcy1sb25nLWFiY2Q=" //nolint:gosec // G101: test material.
	// shedLimit is the admission threshold the server under test runs with. Small on purpose:
	// RT-6 has to reach it on whatever hardware the nightly gets, and what is under test is that
	// the mechanism engages and holds the interactive path - not what the threshold's value
	// should be on real iron, which is the capacity ramp's question.
	shedLimit = 8
	// seedName is the dataset's seed. Named rather than derived from the run, because the
	// baseline the guard compares against was measured over exactly these rows.
	seedName = "hubtask-load"
)

// datasetSize is the dataset the nightly seeds. Two million rows is the figure of the per-release
// capacity ramp on named hardware, seeded by scripts/seed-load-dataset.sh against a real stack;
// carrying them into a container on a shared runner would spend the whole nightly on the COPY and
// measure the runner's disk. So the nightly seeds a scaled dataset and says so, and the two tiers
// stay what decision 7 asks for rather than one tier pretending to be both.
func datasetSize(t *testing.T) (tenants, items int) {
	t.Helper()
	return envInt(t, "HUBTASK_LOAD_TENANTS", 20), envInt(t, "HUBTASK_LOAD_ITEMS", 40_000)
}

func envInt(t *testing.T, name string, fallback int) int {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("%s = %q, which is not a number", name, raw)
	}
	return value
}

// stack is a running installation with a dataset behind it and credentials into it.
type stack struct {
	baseURL string
	// tenants are the workspaces, largest first - which is the order the dataset's long tail is
	// generated in, so tenants[0] is the one holding most of the items.
	tenants []tenant
	// serverPID is the process, so a run can look at what it is costing.
	serverPID int
}

type tenant struct {
	id         string
	token      string
	collection string
}

var (
	sharedStack *stack
	sharedOnce  sync.Once
	sharedErr   error
	// teardown is what the stack leaves behind, run by TestMain after every test in the package.
	// Not t.Cleanup: the stack is brought up by whichever test asks for it first, and cleanups
	// registered on that test's t would tear the installation down while the next test was still
	// using it - which is exactly what the first run of the storm found, in the shape of a rule
	// it could not arm because nothing was listening any more.
	teardown []func()
)

// TestMain owns the installation's lifetime, because it is the only thing here that outlives a
// test.
func TestMain(m *testing.M) {
	code := m.Run()
	for i := len(teardown) - 1; i >= 0; i-- {
		teardown[i]()
	}
	os.Exit(code)
}

// runningStack brings the installation up once for the package. Every run in here wants the same
// dataset, and seeding it per test would spend the nightly on COPY.
func runningStack(t *testing.T) *stack {
	t.Helper()
	tenants, items := datasetSize(t)
	sharedOnce.Do(func() { sharedStack, sharedErr = startStack(tenants, items) })
	if sharedErr != nil {
		t.Fatalf("no installation to put load on: %v", sharedErr)
	}
	return sharedStack
}

func startStack(tenants, items int) (*stack, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	container, appDSN, err := startDatabase(ctx)
	if err != nil {
		return nil, err
	}
	teardown = append(teardown, func() { _ = testcontainers.TerminateContainer(container) })

	if err := seed(ctx, container, tenants, items); err != nil {
		return nil, fmt.Errorf("seeding: %w", err)
	}

	// The credentials are written straight into access_token, the way the end-to-end session
	// bootstraps its own: there is no endpoint that issues a first one, and both halves come from
	// the real constructions so that a token this test hashed its own way could not pass.
	minted, err := mintTokens(ctx, container, tenants)
	if err != nil {
		return nil, fmt.Errorf("minting: %w", err)
	}

	url, pid, err := startServer(ctx, appDSN)
	if err != nil {
		return nil, err
	}

	return &stack{baseURL: url, tenants: minted, serverPID: pid}, nil
}

func postgresImage() string {
	if image := os.Getenv("HUBTASK_TEST_POSTGRES_IMAGE"); image != "" {
		return image
	}
	return "postgres:16-alpine"
}

func startDatabase(ctx context.Context) (*tcpostgres.PostgresContainer, string, error) {
	container, err := tcpostgres.Run(ctx, postgresImage(),
		tcpostgres.WithDatabase("hubtask_load"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute)),
	)
	if err != nil {
		return nil, "", fmt.Errorf("starting the container: %w", err)
	}

	adminDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, "", fmt.Errorf("connection string: %w", err)
	}
	if err := migrate(ctx, adminDSN); err != nil {
		return nil, "", err
	}
	if err := psql(ctx, container,
		"ALTER ROLE hubtask_app WITH LOGIN PASSWORD '"+appPassword+"'"); err != nil {
		return nil, "", fmt.Errorf("granting login: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, "", err
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return nil, "", err
	}
	appDSN := fmt.Sprintf("postgres://hubtask_app:%s@%s:%s/hubtask_load?sslmode=disable",
		appPassword, host, port.Port())
	return container, appDSN, nil
}

func migrate(ctx context.Context, dsn string) error {
	goose := filepath.Join(repositoryRoot(), ".tools", "goose")
	if _, err := os.Stat(goose); err != nil {
		return fmt.Errorf("goose is missing - run 'make tools': %w", err)
	}
	cmd := exec.CommandContext(ctx, goose,
		"-dir", filepath.Join(repositoryRoot(), "db", "migrations"), "postgres", dsn, "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("goose up: %w: %s", err, out)
	}
	return nil
}

// psql runs one statement inside the container, which is how this package stays free of a database
// driver: the tenant boundary is test/integration's subject, and a package that could open its own
// connection could quietly stop going through the application.
func psql(ctx context.Context, container *tcpostgres.PostgresContainer, statement string) error {
	code, reader, err := container.Exec(ctx, []string{
		"psql", "-U", "postgres", "-d", "hubtask_load", "-v", "ON_ERROR_STOP=1", "-c", statement,
	})
	if err != nil {
		return err
	}
	if code != 0 {
		out, _ := io.ReadAll(reader)
		return fmt.Errorf("psql exited %d: %s", code, out)
	}
	return nil
}

// seed writes the dataset with the same generator and the same columns the script uses, so that
// the nightly's scaled dataset and the release's two million are the same rows in different
// numbers rather than two different datasets.
func seed(ctx context.Context, container *tcpostgres.PostgresContainer, tenants, items int) error {
	tables := []struct{ name, columns string }{
		{"tenant", "id, slug, display_name"},
		{"account", "id, tenant_id, kind, display_name, status"},
		{"membership", "id, tenant_id, account_id, scope_type, role"},
		{"container", "id, tenant_id, type, parent_id, name, order_key, created_by"},
		{"work_item", "id, tenant_id, collection_id, type, parent_id, path, depth, title, " +
			"is_completed, completed_at, order_key, due_at, created_by, created_at"},
	}

	for _, table := range tables {
		rows, err := generate(ctx, table.name, tenants, items)
		if err != nil {
			return err
		}
		target := "/tmp/" + table.name + ".tsv"
		// Readable by everyone inside the container on purpose: COPY FROM is read by the server
		// process, which runs as postgres, while the file arrives owned by root.
		if err := container.CopyToContainer(ctx, rows, target, 0o644); err != nil {
			return fmt.Errorf("copying %s: %w", table.name, err)
		}
		if err := psql(ctx, container,
			fmt.Sprintf("COPY %s (%s) FROM '%s'", table.name, table.columns, target)); err != nil {
			return fmt.Errorf("loading %s: %w", table.name, err)
		}
	}
	// The planner needs statistics over the new rows. A ramp against a table PostgreSQL believes
	// is empty measures the wrong plan rather than the hardware.
	return psql(ctx, container, "ANALYZE tenant, account, membership, container, work_item")
}

func generate(ctx context.Context, table string, tenants, items int) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "go", "run", "./test/load/seed",
		"--table", table,
		"--tenants", strconv.Itoa(tenants),
		"--items", strconv.Itoa(items),
		"--seed", seedName)
	cmd.Dir = repositoryRoot()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("generating %s: %w", table, err)
	}
	return out, nil
}

// mintTokens gives the first few tenants a credential each. Several, because the load is spread
// over them: one credential driving the whole run would meet the per-token budget rather than the
// admission threshold, and would measure the rate limiter.
func mintTokens(ctx context.Context, container *tcpostgres.PostgresContainer, tenants int) ([]tenant, error) {
	// automation:manage is here for the storm, which arms a rule and a webhook subscription on the
	// storming tenant; everything else in this package needs only the first three.
	const scopes = "'items:read','items:write','containers:read','automation:manage'"

	credentialled := min(tenants, 8)
	minted := make([]tenant, 0, credentialled)
	hasher := security.NewTokenHasher(secret.New(installationSecret))

	for rank := range credentialled {
		tenantID := derived("tenant", rank)
		accountID := derived("account", rank)

		parsed, err := shared.ParseID(tenantID)
		if err != nil {
			return nil, err
		}
		material := make([]byte, identity.TokenSecretBytes)
		if _, err := rand.Read(material); err != nil {
			return nil, err
		}
		token, err := identity.NewToken(parsed, material)
		if err != nil {
			return nil, err
		}

		if err := psql(ctx, container, fmt.Sprintf(
			`INSERT INTO access_token (id, tenant_id, account_id, name, token_hash, token_prefix, scopes, expires_at)
			 VALUES ('%s', '%s', '%s', 'the load run', decode('%x', 'hex'), 'hbt_pat_',
			         ARRAY[%s], now() + interval '2 hours')`,
			derived("load-token", rank), tenantID, accountID, hasher.Hash(token.Secret()), scopes,
		)); err != nil {
			return nil, err
		}

		minted = append(minted, tenant{
			id:         tenantID,
			token:      token.Secret(),
			collection: derived("collection", rank, 0),
		})
	}
	return minted, nil
}

// startServer builds and runs the real binary rather than wiring the adapters by hand. RT-6 asks
// whether the process holds - its memory, its garbage collector, its pool - and a stack assembled
// in a test is a different process from the one that is deployed.
func startServer(ctx context.Context, dsn string) (string, int, error) {
	home, err := os.MkdirTemp("", "hubtask-load")
	if err != nil {
		return "", 0, err
	}
	teardown = append(teardown, func() { _ = os.RemoveAll(home) })

	binary := filepath.Join(home, "hubtask-server")
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binary, "./cmd/server")
	build.Dir = repositoryRoot()
	if out, err := build.CombinedOutput(); err != nil {
		return "", 0, fmt.Errorf("building the server: %w: %s", err, out)
	}

	httpPort, err := freePort()
	if err != nil {
		return "", 0, err
	}
	opsPort, err := freePort()
	if err != nil {
		return "", 0, err
	}

	// Started with its own context rather than the caller's: the stack outlives the function that
	// brought it up, and t.Cleanup is what ends it.
	serverCtx, stop := context.WithCancel(context.Background())
	server := exec.CommandContext(serverCtx, binary)
	server.Env = append(os.Environ(),
		// Every role in one process, which is what the storm needs: a bulk write that fans into
		// rules, webhooks and the outbox produces nothing visible if nothing is dispatching them.
		// It is also the harsher shape for RT-6 - the background work competes for the same pool
		// as the requests, which is the situation the bulkheads exist for (ADR-0014, ADR-0016).
		"HUBTASK_ROLES=api,worker,scheduler,automation",
		"HUBTASK_DB_DSN="+dsn,
		"HUBTASK_SECRET_KEY="+installationSecret,
		// The storm's webhook subscription carries a signing secret, and the application never
		// stores a plaintext one (E-02): without a key ring, creating one answers 503 rather than
		// silently storing something readable. Fixed material, because the installation and
		// everything sealed under it end together.
		"HUBTASK_ENCRYPTION_KEYS=k1",
		"HUBTASK_ENCRYPTION_KEY_K1="+encryptionKey,
		"HUBTASK_HTTP_ADDR=127.0.0.1:"+strconv.Itoa(httpPort),
		"HUBTASK_OPS_ADDR=127.0.0.1:"+strconv.Itoa(opsPort),
		"HUBTASK_TENANCY_MODE=multi",
		"HUBTASK_LOG_LEVEL=warn",
		"HUBTASK_UI_ENABLED=false",
		"HUBTASK_LOAD_SHED_INFLIGHT="+strconv.Itoa(shedLimit),
		// The rate limits are raised out of the way on purpose. They are admission control of a
		// different kind, they are proved by their own tests, and traffic that runs into them
		// never reaches the threshold this run is about: a 429 is a correct answer that measures
		// the limiter. What is deliberately *not* raised is the pool, which is the capacity the
		// shedding stands in front of.
		"HUBTASK_RATE_LIMIT_ANONYMOUS_PER_MINUTE=1000000",
		"HUBTASK_RATE_LIMIT_TOKEN_PER_MINUTE=1000000",
		"HUBTASK_RATE_LIMIT_TENANT_PER_MINUTE=1000000",
		"HUBTASK_RATE_LIMIT_AUTH_PER_MINUTE=1000000",
		"HUBTASK_RATE_LIMIT_BURST=100000",
		// A limit rather than none, because "no OOM" is only a claim if there is a ceiling to
		// hold: the run asserts the process stays under it (observability-reliability.md §6).
		"GOMEMLIMIT="+strconv.Itoa(memoryLimitBytes),
		// The storm points a webhook subscription at a sink inside the test process, which is a
		// private address the egress guard refuses by default and rightly so (T-07). Opened here
		// by exact host, for a target this run brought up itself.
		"HUBTASK_HTTP_ALLOW_PRIVATE_NETWORKS=true",
		"HUBTASK_HTTP_ALLOWED_HOSTS=127.0.0.1",
	)
	server.Stdout = os.Stderr
	server.Stderr = os.Stderr
	if err := server.Start(); err != nil {
		stop()
		return "", 0, fmt.Errorf("starting the server: %w", err)
	}
	teardown = append(teardown, func() {
		stop()
		_ = server.Wait()
	})

	url := "http://127.0.0.1:" + strconv.Itoa(httpPort)
	if err := waitReady(ctx, "http://127.0.0.1:"+strconv.Itoa(opsPort)+"/readyz"); err != nil {
		return "", 0, err
	}
	return url, server.Process.Pid, nil
}

// memoryLimitBytes is the ceiling the run holds the process to. 768 MiB is the chart's own limit
// for the API role rounded down (k8s/values.yaml), so the number under test is a number somebody
// deploys rather than one invented here.
const memoryLimitBytes = 768 << 20

func freePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitReady(ctx context.Context, url string) error {
	deadline := time.Now().Add(90 * time.Second)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("the server was not ready within the deadline")
}

// derived mirrors the generator's derivation, so that the test can address a row the generator
// wrote without a lookup. It shells out to the generator rather than reimplementing the hash: two
// implementations of the same derivation is exactly how a dataset and the credentials into it
// drift apart.
func derived(kind string, index ...int) string {
	args := []string{"run", "./test/load/seed", "--derive", kind}
	for _, n := range index {
		args = append(args, strconv.Itoa(n))
	}
	cmd := exec.Command("go", args...) //nolint:gosec // G204: the arguments are integers and a constant.
	cmd.Dir = repositoryRoot()
	out, err := cmd.Output()
	if err != nil {
		panic(fmt.Sprintf("deriving %s: %v", kind, err))
	}
	return strings.TrimSpace(string(out))
}

func repositoryRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("go.mod not found above the working directory")
		}
		dir = parent
	}
}

// residentBytes is what the process is costing right now. Read from /proc, which means the memory
// half of RT-6 is asserted where the nightly runs it and reported as unknown elsewhere - a number
// this cannot read is better said than guessed.
func residentBytes(pid int) (int64, bool) {
	status, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(status), "\n") {
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kilobytes, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return kilobytes * 1024, true
	}
	return 0, false
}

// envEvidence is where a run leaves its numbers. The nightly points it at a directory it uploads
// as an artefact; without it the finding goes to the test's own temporary directory and the path
// is logged.
//
// Not docs/evidence/ directly, and that is deliberate: a test that writes into the repository
// leaves the working tree dirty, and a dirty tree is how a gate that compares generated output
// against the checkout goes red for a reason nobody can find. What belongs in docs/evidence/ is a
// run somebody chose to record, put there by hand with the sentences that make it evidence.
const envEvidence = "HUBTASK_LOAD_EVIDENCE_DIR"

// writeEvidence puts the run's own numbers somewhere they survive the process, so that a nightly
// failure is read from the artefact rather than reconstructed from a log.
func writeEvidence(t *testing.T, name string, finding any) {
	t.Helper()
	encoded, err := json.MarshalIndent(finding, "", "  ")
	if err != nil {
		t.Fatalf("encoding the finding: %v", err)
	}

	directory := os.Getenv(envEvidence)
	if directory == "" {
		directory = t.TempDir()
	} else if err := os.MkdirAll(directory, 0o750); err != nil {
		t.Fatalf("making %s: %v", directory, err)
	}

	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	t.Logf("the run is in %s", path)
}
