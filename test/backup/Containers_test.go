// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

// Package backup is BK-1: one conformance suite, run against every adapter this build has
// (ADR-0019 decision 2).
//
// The suite is the gate a fifth adapter passes before it exists. That is the whole argument for
// running it against real servers rather than against fakes: a hand-written SFTP client is worth
// exactly as much as the OpenSSH that accepts it, and a hand-written SigV4 signature is worth
// exactly as much as MinIO's strict validation of it. A fake that agreed with the adapter would
// prove that the adapter agrees with itself.
package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Jersyfi/hubtask/core/domain/model/backup"
	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// The images, each overridable the way the PostgreSQL and MinIO ones already are, so the support
// matrix can vary them without a code change.
func imageOr(variable, fallback string) string {
	if image := os.Getenv(variable); image != "" {
		return image
	}
	return fallback
}

// outbound is the configuration these tests need. Every container is on loopback, which the guard
// blocks by design and correctly - a backup target on a private network needs exactly this release
// in production, which is the point rather than a workaround (BK-9 proves the other direction).
func outbound() env.OutboundConfig {
	return env.OutboundConfig{
		Timeout: 60 * time.Second, ConnectTimeout: 5 * time.Second,
		MaxResponseBytes: 1 << 20, MaxRedirects: 0, AllowPrivateNetworks: true,
	}
}

func registry(t *testing.T, localRoot string) backupstorage.Registry {
	t.Helper()
	cfg := outbound()
	guard := httpclient.NewGuard(cfg)
	return backupstorage.NewRegistry(
		httpclient.NewGuardedClient(cfg, guard), guard, localRoot, 10*time.Second, time.Now)
}

// startMinIO runs one MinIO and answers the endpoint it listens on, with the bucket made.
func startMinIO(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: imageOr("HUBTASK_TEST_MINIO_IMAGE", "minio/minio:latest"),
			Env: map[string]string{
				"MINIO_ROOT_USER":     "conformance",
				"MINIO_ROOT_PASSWORD": "conformance-secret",
			},
			Cmd:          []string{"server", "/data"},
			ExposedPorts: []string{"9000/tcp"},
			WaitingFor: wait.ForHTTP("/minio/health/ready").
				WithPort("9000/tcp").WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("starting MinIO: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	endpoint := "http://" + hostPort(t, ctx, container, "9000/tcp")

	// The bucket, made through MinIO's own client rather than by this adapter: creating a bucket
	// is not something a backup target does, and an adapter that could would be an adapter that
	// can create a bucket by mistake.
	code, output, err := container.Exec(ctx, []string{
		"sh", "-c",
		"mc alias set local http://127.0.0.1:9000 conformance conformance-secret && " +
			"mc mb --ignore-existing local/hubtask-backups",
	})
	if err != nil || code != 0 {
		body, _ := readAll(output)
		t.Fatalf("creating the bucket: %v (exit %d) %s", err, code, body)
	}
	return endpoint
}

// startWebDAV runs an Apache with the DAV module on. Apache deliberately, and not one of the
// purpose-built images: its `DavDepthInfinity off` default is what proves the adapter recurses at
// depth one rather than asking for a depth Apache refuses.
func startWebDAV(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: imageOr("HUBTASK_TEST_HTTPD_IMAGE", "httpd:2.4-alpine"),
			Files: []testcontainers.ContainerFile{{
				Reader:            strings.NewReader(davConfiguration),
				ContainerFilePath: "/usr/local/apache2/conf/httpd.conf",
				FileMode:          0o644,
			}},
			Entrypoint: []string{"sh", "-c",
				"mkdir -p /var/dav && chown -R daemon:daemon /var/dav && httpd-foreground"},
			ExposedPorts: []string{"80/tcp"},
			WaitingFor: wait.ForHTTP("/").WithPort("80/tcp").
				WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("starting the WebDAV server: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	return "http://" + hostPort(t, ctx, container, "80/tcp") + "/"
}

// startSFTP runs an OpenSSH with the SFTP subsystem, and reads its host key out of the container
// - because this adapter refuses to connect without one, which is the property under test as much
// as the protocol is.
func startSFTP(t *testing.T) (address, hostKey string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        imageOr("HUBTASK_TEST_SFTP_IMAGE", "atmoz/sftp:alpine"),
			Cmd:          []string{"hubtask:conformance-secret:::backups"},
			ExposedPorts: []string{"22/tcp"},
			WaitingFor: wait.ForLog("Server listening on").
				WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("starting the SFTP server: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	code, output, err := container.Exec(ctx,
		[]string{"cat", "/etc/ssh/ssh_host_ed25519_key.pub"})
	if err != nil || code != 0 {
		t.Fatalf("reading the host key: %v (exit %d)", err, code)
	}
	key, err := readAll(output)
	if err != nil {
		t.Fatalf("reading the host key: %v", err)
	}
	return hostPort(t, ctx, container, "22/tcp"), strings.TrimSpace(stripDockerFrames(key))
}

func hostPort(
	t *testing.T, ctx context.Context, container testcontainers.Container, exposed string,
) string {
	t.Helper()

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("reading the container's host: %v", err)
	}
	mapped, err := container.MappedPort(ctx, exposed)
	if err != nil {
		t.Fatalf("reading the container's port: %v", err)
	}
	return fmt.Sprintf("%s:%s", host, mapped.Port())
}

// specFor builds the target specification each adapter is opened with.
func specFor(kind backup.TargetKind, config backup.TargetConfig, credentials map[string]string) port.Spec {
	wrapped := map[string]secret.Secret{}
	for name, value := range credentials {
		wrapped[name] = secret.New(value)
	}
	return port.Spec{Kind: kind, Config: config, Credentials: wrapped}
}

// davConfiguration is a complete Apache configuration with the DAV module on and nothing else.
// Written out rather than patched into the image's default, so that what the server does is
// visible in this file.
const davConfiguration = `
ServerRoot "/usr/local/apache2"
Listen 80
LoadModule mpm_event_module modules/mod_mpm_event.so
LoadModule authn_core_module modules/mod_authn_core.so
LoadModule authz_core_module modules/mod_authz_core.so
LoadModule unixd_module modules/mod_unixd.so
LoadModule log_config_module modules/mod_log_config.so
LoadModule mime_module modules/mod_mime.so
LoadModule dir_module modules/mod_dir.so
LoadModule dav_module modules/mod_dav.so
LoadModule dav_fs_module modules/mod_dav_fs.so
User daemon
Group daemon
ServerName localhost
ErrorLog /proc/self/fd/2
LogLevel warn
DocumentRoot "/var/dav"
DavLockDB /tmp/DavLock
<Directory "/var/dav">
    Dav On
    Options Indexes
    AllowOverride None
    Require all granted
</Directory>
`

// readAll drains what a container exec answered.
func readAll(from io.Reader) (string, error) {
	body, err := io.ReadAll(from)
	return string(body), err
}

// stripDockerFrames removes the eight-byte headers the Docker exec API interleaves with the
// output. A host key read through it would otherwise carry them into the configuration.
func stripDockerFrames(raw string) string {
	var out strings.Builder
	for i := 0; i < len(raw); {
		if len(raw)-i >= 8 && raw[i] <= 2 && raw[i+1] == 0 && raw[i+2] == 0 && raw[i+3] == 0 {
			size := int(raw[i+4])<<24 | int(raw[i+5])<<16 | int(raw[i+6])<<8 | int(raw[i+7])
			i += 8
			if size > len(raw)-i {
				size = len(raw) - i
			}
			out.WriteString(raw[i : i+size])
			i += size
			continue
		}
		out.WriteByte(raw[i])
		i++
	}
	return out.String()
}
