// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration || resilience

// Package s3test starts the S3-compatible server every suite that needs a real one runs against,
// the way test/dbtest starts the PostgreSQL.
//
// One place rather than three, because the previous arrangement was three copies of the same
// image pin plus two more in scripts/pitr-drill.sh - and when the vendor closed its registries
// that made a one-line problem into a five-place one (#1029).
//
// Two build tags, because two gates need it: the storage and backup suites run under
// `integration` and RT-1 runs under `resilience`. test/dbtest needs only the first.
//
// **SeaweedFS rather than MinIO.** MinIO archived its open-source server, client and KES: the
// GitHub repository is archived, dl.min.io answers 410 Gone to every path, and Docker Hub and
// quay.io both stopped serving `minio/minio` anonymously. There is no fallback registry left to
// move to, and no security update will ever be published for the pinned release.
//
// The replacement was chosen by measurement rather than reputation, against the assertions these
// suites actually make: the presigned window is enforced by the server (403, `Request has
// expired`), a tampered signature is refused (403, `SignatureDoesNotMatch`), a wrong secret key
// is refused, and `response-content-disposition` is honoured. SeaweedFS answers all of them with
// the same status codes MinIO did, which is why no assertion in the suites had to be relaxed.
// Garage and RustFS enforce the same rules but answer 400 where MinIO answers 403, and RustFS has
// no stable release. Apache-2.0 also makes the image lawful to mirror, which AGPL-3.0 did not.
package s3test

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/wait"
)

// The credential and the region the suites sign with. Fixed and not secret: they exist for the
// length of one container, and the strict validation of them is the point.
//
// us-east-1 deliberately: CreateBucket sends no location constraint, and this is the region for
// which none is needed.
const (
	AccessKey = "conformance"
	SecretKey = "conformance-not-a-secret"
	Region    = "us-east-1"

	// Port is the S3 API. SeaweedFS serves the master on 9333 and the filer on 8888 as well;
	// nothing here talks to either, and neither is exposed.
	Port = "8333/tcp"
)

// identities is the server's whole authorisation surface: one key that may do everything inside
// this container. Anything narrower would be a second thing under test.
func identities() string {
	return fmt.Sprintf(
		`{"identities":[{"name":%q,"credentials":[{"accessKey":%q,"secretKey":%q}],`+
			`"actions":["Admin","Read","Write","List","Tagging"]}]}`,
		AccessKey, AccessKey, SecretKey)
}

// Image is the server the suites run against, overridable the way the PostgreSQL image is
// (test/dbtest), so the support matrix can vary it without a code change.
func Image() string {
	if image := os.Getenv("HUBTASK_TEST_S3_IMAGE"); image != "" {
		return image
	}
	return "chrislusf/seaweedfs:4.47"
}

// Request is the container, without the parts a caller owns: RT-1 binds it to a fixed host port
// so that an endpoint survives a restart, and the other two let the daemon choose.
//
// `/healthz` rather than the S3 root, and it is a real readiness promise rather than a liveness
// one: the bucket creation that follows is a write, and MinIO's `ready` answering before write
// quorum is what made the old suite flake. Measured here over five cold starts - a write straight
// after the first 200 succeeded every time.
func Request() testcontainers.ContainerRequest {
	return testcontainers.ContainerRequest{
		Image: Image(),
		// One process serving master, volume, filer and S3. `-dir` keeps its state inside the
		// container, which is what lets RT-1 stop and start it and find its objects again.
		Cmd: []string{"server", "-s3", "-s3.config=/etc/seaweedfs/s3.json", "-dir=/data"},
		Files: []testcontainers.ContainerFile{{
			Reader:            strings.NewReader(identities()),
			ContainerFilePath: "/etc/seaweedfs/s3.json",
			FileMode:          0o644,
		}},
		ExposedPorts: []string{Port},
		WaitingFor: wait.ForHTTP("/healthz").
			WithPort(Port).WithStartupTimeout(2 * time.Minute),
	}
}

// CreateBucket makes a bucket through the server's own shell, for the suite that needs one to
// exist before the adapter under test is pointed at it - creating a bucket is not something a
// backup target does, and an adapter that could would be an adapter that can create one by
// mistake.
//
// The result is read back rather than trusted. `weed shell` exits 0 on an unknown command and
// prints the complaint instead, and it blocks indefinitely when it cannot reach the master, so
// neither the exit code nor the call returning is evidence that the bucket exists. The deadline
// is this function's, not the caller's, for the same reason.
//
// `Multiplexed` is not optional: without it the reader hands back Docker's attach stream, whose
// eight-byte frame headers land in the middle of the text as stray characters. It reads as a
// shell prompt glued to the first line, and it breaks any check that looks at where a word sits.
func CreateBucket(ctx context.Context, container testcontainers.Container, name string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	code, output, err := container.Exec(ctx, []string{"sh", "-c",
		fmt.Sprintf("printf 's3.bucket.create -name %s\\ns3.bucket.list\\n' | "+
			"weed shell -master=localhost:9333", name)}, tcexec.Multiplexed())
	if err != nil {
		return fmt.Errorf("creating the bucket %q: %w", name, err)
	}
	body, readErr := io.ReadAll(output)
	if readErr != nil {
		return fmt.Errorf("reading the shell's answer: %w", readErr)
	}
	if code != 0 {
		return fmt.Errorf("creating the bucket %q: exit %d: %s", name, code, body)
	}
	// The listing is the proof: the name is there, or the create did not happen.
	if !listed(string(body), name) {
		return fmt.Errorf("the bucket %q is absent from the listing after creating it: %s", name, body)
	}
	return nil
}

// listed answers whether the shell's output names this bucket, by the whole name rather than by a
// substring of one. `s3.bucket.list` prints one indented name per line followed by its sizes, and
// a bucket called `media` would otherwise be found in a listing that only holds `hubtask-media` -
// or in the refusal the server prints when it declines the name, which `weed shell` follows with
// exit 0.
func listed(output, name string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == name {
			return true
		}
	}
	return false
}

// Start runs one server for this test and answers the container, with the bucket made when one is
// named. The caller turns it into an endpoint; what varies between the suites is what they do with
// it, not how it starts.
func Start(t *testing.T, bucket string) testcontainers.Container {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: Request(),
		Started:          true,
	})
	if err != nil {
		t.Fatalf("starting %s: %v", Image(), err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	if bucket != "" {
		if err := CreateBucket(ctx, container, bucket); err != nil {
			t.Fatalf("%v", err)
		}
	}
	return container
}

// Endpoint is the URL the adapters are pointed at.
func Endpoint(ctx context.Context, t *testing.T, container testcontainers.Container) string {
	t.Helper()
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("the container's host: %v", err)
	}
	port, err := container.MappedPort(ctx, Port)
	if err != nil {
		t.Fatalf("the container's mapped port: %v", err)
	}
	return fmt.Sprintf("http://%s:%s", host, port.Port())
}
