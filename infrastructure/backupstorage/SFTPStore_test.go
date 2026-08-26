// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backupstorage_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// The configuration surface of the SFTP target, which is where its security is decided. That the
// protocol itself works is BK-1's job against a real server - a hand-written client is worth
// exactly as much as the server that accepts it.

// A public key of the right shape, so that the "configured and matching" path can be built
// without a server. It is a published test vector rather than anybody's key.
const sampleHostKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGb9ECWmEzf6FQbrBZ9w7lshQhqowDY5hpOF5dSFXYlA"

func sftpSpec(mutate ...func(*port.Spec)) port.Spec {
	spec := port.Spec{
		Kind: backup.KindSFTP,
		Config: backup.TargetConfig{
			"host": "backup.example.org", "path": "/srv/hubtask",
			"username": "hubtask", "host_key": sampleHostKey,
		},
		Credentials: map[string]secret.Secret{"password": secret.New("the-sftp-password")},
	}
	for _, m := range mutate {
		m(&spec)
	}
	return spec
}

func sftpStore(t *testing.T, spec port.Spec) (*backupstorage.SFTPStore, error) {
	t.Helper()
	cfg := permissive()
	return backupstorage.NewSFTPStore(httpclient.NewGuard(cfg), spec, time.Second)
}

// The decision this adapter would otherwise get quietly wrong. There is no trust on first use and
// no switch: a target is created through an API, possibly by somebody who is not the operator, and
// a first connection that accepted whatever answered is one an attacker only has to be present
// for once.
func TestATargetWithNoHostKeyIsRefused(t *testing.T) {
	_, err := sftpStore(t, sftpSpec(func(spec *port.Spec) {
		delete(spec.Config, "host_key")
	}))
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a target with no host key was accepted: %v", err)
	}

	var named bool
	for _, field := range shared.AsError(err).Fields {
		if field.Path == "/config/host_key" && field.Code == "backup.host_key_required" {
			named = true
		}
	}
	if !named {
		t.Fatalf("the refusal says %v", shared.AsError(err).Fields)
	}
}

// Two spellings, because operators have one or the other to hand: the whole public key as
// ssh-keyscan prints it, or the fingerprint as ssh-keygen -lf does.
func TestBothSpellingsOfAHostKeyAreAccepted(t *testing.T) {
	if _, err := sftpStore(t, sftpSpec()); err != nil {
		t.Fatalf("a target naming the whole key was refused: %v", err)
	}

	_, err := sftpStore(t, sftpSpec(func(spec *port.Spec) {
		delete(spec.Config, "host_key")
		spec.Config["host_key_fingerprint"] = "SHA256:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU"
	}))
	if err != nil {
		t.Fatalf("a target naming the fingerprint was refused: %v", err)
	}
}

func TestAHostKeyThatIsNotOneIsRefused(t *testing.T) {
	cases := map[string]func(*port.Spec){
		"a key that does not parse": func(spec *port.Spec) {
			spec.Config["host_key"] = "not a key"
		},
		"a fingerprint in the old format": func(spec *port.Spec) {
			delete(spec.Config, "host_key")
			spec.Config["host_key_fingerprint"] = "MD5:16:27:ac:a5:76:28:2d:36:63:1b:56:4d:eb:df:a6:48"
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := sftpStore(t, sftpSpec(mutate)); !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("accepted: %v", err)
			}
		})
	}
}

func TestATargetWithNoWayToAuthenticateIsRefused(t *testing.T) {
	_, err := sftpStore(t, sftpSpec(func(spec *port.Spec) { spec.Credentials = nil }))
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a target with no credential was accepted: %v", err)
	}
}

// A private key that does not parse is a field error, and the parser's message never travels with
// it: it can quote the key's header and, on some paths, its body.
func TestAPrivateKeyThatDoesNotParseIsAFieldErrorAndQuotesNothing(t *testing.T) {
	broken := "-----BEGIN OPENSSH PRIVATE KEY-----\nthe-secret-material\n-----END OPENSSH PRIVATE KEY-----"

	_, err := sftpStore(t, sftpSpec(func(spec *port.Spec) {
		spec.Credentials = map[string]secret.Secret{"private_key": secret.New(broken)}
	}))
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a key that does not parse was accepted: %v", err)
	}
	if got := err.Error(); strings.Contains(got, "the-secret-material") {
		t.Fatalf("the refusal quoted the key: %s", got)
	}
}

func TestATargetWithNowhereToWriteIsRefused(t *testing.T) {
	for _, setting := range []string{"host", "path"} {
		_, err := sftpStore(t, sftpSpec(func(spec *port.Spec) { delete(spec.Config, setting) }))
		if !errors.Is(err, shared.ErrValidation) {
			t.Errorf("a target with no %s was accepted: %v", setting, err)
		}
	}
}

// The guard applies to SSH as it does to HTTP. Rule 6's wording covers HTTP, but the reason behind
// it does not stop at a scheme: 169.254.169.254 answers on port 22 as readily as on port 80 if
// somebody puts a listener there.
func TestATargetOnALinkLocalAddressIsBlocked(t *testing.T) {
	strict := permissive()
	strict.AllowPrivateNetworks = false

	store, err := backupstorage.NewSFTPStore(
		httpclient.NewGuard(strict),
		sftpSpec(func(spec *port.Spec) { spec.Config["host"] = "169.254.169.254" }),
		time.Second)
	if err != nil {
		t.Fatalf("opening the target: %v", err)
	}

	_, err = store.Stat(t.Context(), "a.hbk")
	if err == nil {
		t.Fatal("the metadata endpoint was reached")
	}
	if !httpclient.IsBlocked(err) {
		t.Fatalf("it was refused for the wrong reason: %v", err)
	}
}
