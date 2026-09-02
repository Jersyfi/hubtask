// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package eventbus

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The subject is where routing happens in NATS, which is why the tenant is a token in it and not
// only an attribute: a consumer wanting one workspace binds `<prefix>.<id>.>` instead of filtering
// every message it receives.
func TestTheSubjectCarriesThePrefixTheTenantAndTheType(t *testing.T) {
	tenant := shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")

	got := Subject("hubtask", tenant, "de.hubtask.work.item.created.v1")
	if want := "hubtask." + tenant.String() + ".work.item.created.v1"; got != want {
		t.Errorf("subject = %q, want %q", got, want)
	}
}

// The `de.hubtask.` namespace is stripped because the configured prefix already namespaces the
// stream. Repeating it would make every subject four tokens longer for no routing anybody can use.
func TestTheSubjectDoesNotRepeatTheNamespace(t *testing.T) {
	subject := Subject("hubtask", shared.ID("01936f2a-7c1e-7000-8000-0000000000a1"),
		"de.hubtask.automation.rule_run.finished.v1")

	if strings.Contains(subject, "de.hubtask") {
		t.Errorf("subject = %q, which carries the namespace twice", subject)
	}
	if !strings.HasSuffix(subject, ".automation.rule_run.finished.v1") {
		t.Errorf("subject = %q", subject)
	}
}

// A type from outside the namespace is passed through whole rather than mangled. Nothing emits one
// today; what this pins is that the stripping is a prefix trim and not a "drop the first two
// tokens", which would silently truncate the day something else does.
func TestASubjectOutsideTheNamespaceIsNotTruncated(t *testing.T) {
	subject := Subject("hubtask", shared.ID("01936f2a-7c1e-7000-8000-0000000000a1"), "other.thing.v1")

	if !strings.HasSuffix(subject, ".other.thing.v1") {
		t.Errorf("subject = %q", subject)
	}
}

// A publisher with no connection answers UNAVAILABLE rather than panicking or blocking. It is the
// state a bus that is down at startup leaves behind, and the job's retry ladder is what happens
// next.
func TestAPublisherWithNoConnectionIsUnavailable(t *testing.T) {
	publisher := &Publisher{}

	err := publisher.Publish(t.Context(), shared.ID("01936f2a-7c1e-7000-8000-0000000000a1"),
		"de.hubtask.work.item.created.v1", []byte("{}"))
	if err == nil {
		t.Fatal("publishing without a connection was reported as a success")
	}
	if domainErr := shared.AsError(err); domainErr.Category != shared.CategoryUnavailable {
		t.Errorf("the refusal is a %s", domainErr.Category)
	}
	if publisher.Connected() {
		t.Error("a publisher with no connection reports itself connected")
	}
}
