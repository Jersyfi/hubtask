// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backupstorage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The port carries no protocol, so there is nothing here to measure - only something to hold in
// place. The double proves all three interfaces can be implemented by a fake, which is what the
// use case tests depend on, and that the optional half stays optional.
type double struct{}

func (double) Put(context.Context, string, io.Reader) (int64, error) { return 0, nil }
func (double) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, shared.ErrNotFound.WithDetail(CodeObjectNotFound)
}
func (double) List(context.Context, string) ([]Entry, error) { return nil, nil }
func (double) Stat(context.Context, string) (Entry, error) {
	return Entry{}, shared.ErrNotFound.WithDetail(CodeObjectNotFound)
}
func (double) Delete(context.Context, string) error { return nil }

var _ Store = double{}

// roomy is a store that can also say how much room is left. Most protocols cannot, which is why
// it is a second interface rather than a sixth method.
type roomy struct{ double }

func (roomy) FreeBytes(context.Context) (int64, error) { return 1 << 40, nil }

var _ SpaceReporter = roomy{}

func TestTheSpaceReportIsOptional(t *testing.T) {
	if _, answers := any(double{}).(SpaceReporter); answers {
		t.Error("a store that cannot say how much room is left claims it can")
	}
	reporter, answers := any(roomy{}).(SpaceReporter)
	if !answers {
		t.Fatal("a store that can say how much room is left does not satisfy the interface")
	}
	free, err := reporter.FreeBytes(t.Context())
	if err != nil || free == 0 {
		t.Fatalf("free bytes = %d, %v", free, err)
	}
}

// A missing object is ErrNotFound and never an empty stream. A restore that read nothing where an
// archive should have been would report success over an empty database.
func TestAMissingObjectIsNotAnEmptyStream(t *testing.T) {
	content, err := (double{}).Get(t.Context(), "missing")
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a missing object is reported as %v", err)
	}
	if content != nil {
		t.Error("a missing object came back with a stream")
	}
}

// Deleting what is not there succeeds. Deletion is the state the caller asked for, and the
// generational retention that calls it retries.
func TestDeletingWhatIsNotThereSucceeds(t *testing.T) {
	if err := (double{}).Delete(t.Context(), "missing"); err != nil {
		t.Fatalf("deleting nothing: %v", err)
	}
}

// A specification prints nothing of its credentials, however carelessly it is printed (T-18).
func TestASpecificationPrintsNoCredential(t *testing.T) {
	spec := Spec{
		Kind:   backup.KindS3,
		Config: backup.TargetConfig{"bucket": "hubtask-backups"},
		Credentials: map[string]secret.Secret{
			"access_key": secret.New("AKIAEXAMPLE"),
			"secret_key": secret.New("the-secret-access-key-of-the-bucket"),
		},
	}

	printed := strings.Join([]string{
		formatted(spec), formatted(spec.Credentials), formatted(spec.Credential("secret_key")),
	}, " ")
	for _, plaintext := range []string{"AKIAEXAMPLE", "the-secret-access-key"} {
		if strings.Contains(printed, plaintext) {
			t.Fatalf("printing the specification leaked %q", plaintext)
		}
	}
	// The configuration is not secret and has to be printable: it is what an operator reads back.
	if !strings.Contains(printed, "hubtask-backups") {
		t.Error("the configuration cannot be printed at all")
	}
	if !spec.Credential("nothing_of_that_name").IsEmpty() {
		t.Error("a credential the target does not carry came back as something")
	}
}

func TestAnEntryCarriesWhatAListingNeeds(t *testing.T) {
	entry := Entry{
		Key:        "hubtask/2026/08/archive.hbk",
		Size:       1 << 20,
		ModifiedAt: time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC),
	}
	// The whole key rather than a name relative to the prefix, so a caller can hand it straight
	// back to Get.
	if !strings.HasPrefix(entry.Key, "hubtask/") {
		t.Error("the entry does not carry the whole key")
	}
}

// formatted prints a value the way a careless log line would.
func formatted(value any) string {
	return fmt.Sprintf("%v %+v %#v", value, value, value)
}
