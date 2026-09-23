// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/notification"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The message about a rule that was switched off commits with the real tables and reaches its
// author naming the rule (issue 814). Before, the rule's identifier went into item_id, whose key
// points at work_item: the insert failed, and with it the check's transaction and the streak's -
// which is why the automation list answered 503 whenever a rule was broken. The same write sits
// at the end of both paths (the check's DisableBroken, the streak's settle), so one write proves
// both commit.
func TestARuleDisabledMessageCommitsAndNamesTheRule(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	stack := newNotificationStack(ctx, t)

	address := freshName(t) + "@test.invalid"
	author := seedPerson(ctx, t, tenantB, "Anna", address, "en")
	runAs := seedServiceAccount(ctx, t, tenantB)
	rule := ruleFixture(t, tenantB, runAs, author)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return automationRules().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	owners := notification.RecordRuleDisabled{
		Notifications: postgres.NewNotificationRepository(), Accounts: postgres.NewAccountRepository(),
		Preferences: postgres.NewNotificationPreferenceRepository(),
		Jobs:        postgres.NewQueue(ids, clockadapter.System{}),
		Clock:       clockadapter.System{}, IDs: ids,
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return owners.RuleDisabled(ctx, rule, time.Now().UTC())
	}); err != nil {
		t.Fatalf("recording the disable: %v", err)
	}

	running, stop := context.WithCancel(ctx)
	defer stop()
	concurrency.Go(running, "test.worker", stack.runner.Run)

	var received string
	waitFor(t, 15*time.Second, "the rule's author to be written to", func() bool {
		for _, message := range stack.mail.messages() {
			if strings.Contains(message, "To: "+address) {
				received = message
				return true
			}
		}
		return false
	})
	if subject := subjectOf(t, received); subject != "Hubtask switched off the rule “"+rule.Name+"”" {
		t.Errorf("the subject is %q", subject)
	}
	// The body travels quoted-printable, which folds a long link over a soft line break.
	unfolded := strings.ReplaceAll(strings.ReplaceAll(received, "=\r\n", ""), "=\n", "")
	if !strings.Contains(unfolded, "/administration/rules/"+rule.ID.String()) {
		t.Errorf("the body does not link to the rule:\n%s", received)
	}
}
