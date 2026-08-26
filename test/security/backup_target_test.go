// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Gate BK-9: a backup target is an egress channel somebody configured through an API, and it goes
// through the same guard every other outbound call does (backup-restore.md §2, security.md T-07).
//
// The reason this is a security gate rather than an adapter test is the shape of the mistake it
// prevents. A backup target is the one piece of configuration in this system that names an
// arbitrary host *and* is set by a user rather than by the operator - so it is the obvious way to
// ask this process to fetch a cloud provider's instance credentials, or to enumerate what is
// listening on the network the container sits in.
package security

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/backup"
	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// The addresses that must not be reachable. The first is the one an attacker actually wants: on
// AWS, GCP and Azure alike it answers with credentials to whatever asks from inside the instance.
var blockedHosts = map[string]string{
	"the cloud metadata endpoint":     "169.254.169.254",
	"the same link-local range":       "169.254.42.7",
	"an RFC 1918 address":             "10.11.12.13",
	"the other RFC 1918 range":        "192.168.1.1",
	"loopback, where this process is": "127.0.0.1",
	"loopback by its other name":      "[::1]",
}

func backupOutbound(allowPrivate bool) env.OutboundConfig {
	return env.OutboundConfig{
		Timeout: 2 * time.Second, ConnectTimeout: time.Second,
		MaxResponseBytes: 1 << 20, MaxRedirects: 0, AllowPrivateNetworks: allowPrivate,
	}
}

func adapters(allowPrivate bool) backupstorage.Registry {
	cfg := backupOutbound(allowPrivate)
	guard := httpclient.NewGuard(cfg)
	return backupstorage.NewRegistry(
		httpclient.NewGuardedClient(cfg, guard), guard, "", time.Second, time.Now)
}

// specsFor is the same target address expressed as each of the three kinds that reach a network.
// A LOCAL target reaches no network at all, which is why it is not here.
func specsFor(host string) map[string]port.Spec {
	return map[string]port.Spec{
		"s3": {
			Kind: backup.KindS3,
			Config: backup.TargetConfig{
				"bucket": "hubtask", "endpoint": "http://" + host + ":9000",
			},
			Credentials: map[string]secret.Secret{
				"access_key": secret.New("AKIAEXAMPLE"),
				"secret_key": secret.New("the-secret-access-key"),
			},
		},
		"webdav": {
			Kind:   backup.KindWebDAV,
			Config: backup.TargetConfig{"url": "http://" + host + "/backups"},
		},
		"sftp": {
			Kind: backup.KindSFTP,
			Config: backup.TargetConfig{
				"host": strings.Trim(host, "[]"), "path": "/srv/backups", "username": "hubtask",
				"host_key_fingerprint": "SHA256:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU",
			},
			Credentials: map[string]secret.Secret{"password": secret.New("hunter2")},
		},
	}
}

func TestABackupTargetCannotBePointedIntoTheNetworkItRunsIn(t *testing.T) {
	registry := adapters(false)

	for name, host := range blockedHosts {
		for kind, spec := range specsFor(host) {
			t.Run(name+" over "+kind, func(t *testing.T) {
				store, err := registry.Open(t.Context(), spec)
				if err != nil {
					// Refused before a connection was even described: also a pass, and the
					// earliest one.
					return
				}

				// Every method, because the guard is applied per call and a method that skipped
				// it would be the one an attacker uses. A listing is the one that reads.
				if _, err := store.List(t.Context(), ""); !httpclient.IsBlocked(err) {
					t.Errorf("listing was not blocked: %v", err)
				}
				if _, err := store.Stat(t.Context(), "a.hbk"); !httpclient.IsBlocked(err) {
					t.Errorf("measuring was not blocked: %v", err)
				}
				if _, err := store.Get(t.Context(), "a.hbk"); !httpclient.IsBlocked(err) {
					t.Errorf("reading was not blocked: %v", err)
				}
				if _, err := store.Put(t.Context(), "a.hbk", strings.NewReader("x")); !httpclient.IsBlocked(err) {
					t.Errorf("writing was not blocked: %v", err)
				}
				if err := store.Delete(t.Context(), "a.hbk"); !httpclient.IsBlocked(err) {
					t.Errorf("deleting was not blocked: %v", err)
				}
			})
		}
	}
}

// The other half of the criterion: it is released by explicit configuration and by nothing else.
// A self-hoster whose NAS is on the same LAN is the one legitimate reason, and it is a decision an
// operator makes once for the installation rather than one every target gets for free.
func TestTheReleaseIsExplicitAndNothingElseGrantsIt(t *testing.T) {
	released := adapters(true)

	for kind, spec := range specsFor("10.11.12.13") {
		t.Run(kind, func(t *testing.T) {
			store, err := released.Open(t.Context(), spec)
			if err != nil {
				t.Fatalf("opening: %v", err)
			}

			// Nothing is listening there, so the call fails - but it has to fail as a target
			// that did not answer rather than as one the guard refused. Those are different
			// problems with different fixes, and an operator reading the code has to be able to
			// tell them apart.
			_, err = store.Stat(t.Context(), "a.hbk")
			if err == nil {
				t.Fatal("something answered at an address nothing should be listening on")
			}
			if httpclient.IsBlocked(err) {
				t.Fatalf("the release did not take effect: %v", err)
			}
		})
	}
}

// A URL is not the only way in. A redirect is the classic way around an allowlist, and this client
// follows none at all - which the outbound suite already proves for the guarded client, and which
// the backup adapters inherit rather than re-decide.
func TestABackupTargetFollowsNoRedirect(t *testing.T) {
	cfg := backupOutbound(true)
	if cfg.MaxRedirects != 0 {
		t.Fatalf("the backup adapters are configured for %d redirects", cfg.MaxRedirects)
	}
}
