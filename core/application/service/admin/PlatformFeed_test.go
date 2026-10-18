// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// feedStore is the installation's events as the table keeps them: numbered, read after a number.
type feedStore struct {
	platformLog
	kept []adminrepo.PlatformEvent
}

func (s *feedStore) After(_ context.Context, after int64, limit int) ([]adminrepo.PlatformEvent, error) {
	var page []adminrepo.PlatformEvent
	for _, e := range s.kept {
		if e.Number > after && len(page) < limit {
			page = append(page, e)
		}
	}
	return page, nil
}

// plainCursors stands in for the signed codec: the service never looks inside the string.
type plainCursors struct{}

func (plainCursors) Encode(p adminrepo.PlatformPosition) string {
	return strconv.FormatInt(p.Number, 10) + "|" + strconv.FormatInt(p.At.UnixMicro(), 10)
}

func (plainCursors) Decode(cursor string) (adminrepo.PlatformPosition, error) {
	number, at, ok := strings.Cut(cursor, "|")
	n, err := strconv.ParseInt(number, 10, 64)
	micros, err2 := strconv.ParseInt(at, 10, 64)
	if !ok || err != nil || err2 != nil {
		return adminrepo.PlatformPosition{}, shared.ErrValidation.WithDetail("shared.cursor_invalid")
	}
	return adminrepo.PlatformPosition{Number: n, At: time.UnixMicro(micros).UTC()}, nil
}

type idRendering struct{}

func (idRendering) Render(envelope event.Envelope) map[string]any {
	return map[string]any{"id": envelope.ID.String(), "type": string(envelope.Type)}
}

func feedRegistry(t *testing.T, store *feedStore, tenancy env.TenancyMode, register ...shared.ID) *usecase.Registry {
	t.Helper()
	writer, _, _ := newInstanceWriter(newRegister(register...))
	registry, err := usecase.NewRegistry(nil, ListPlatformEvents{
		Writer: writer, Events: store, Cursors: plainCursors{}, Rendering: idRendering{}, Tenancy: tenancy,
	}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func keptEvents(n int, at time.Time) []adminrepo.PlatformEvent {
	kept := make([]adminrepo.PlatformEvent, 0, n)
	for i := 1; i <= n; i++ {
		kept = append(kept, adminrepo.PlatformEvent{Number: int64(i * 10), Envelope: event.Envelope{
			ID: shared.ID("0192f000-0000-7000-8000-0000000000" + strconv.Itoa(10+i)), Type: event.WorkspaceCreated,
			OccurredAt: at.Add(time.Duration(i) * time.Minute),
		}})
	}
	return kept
}

func pageOf(t *testing.T, out usecase.Output) ([]any, string, bool) {
	t.Helper()
	data, _ := out["data"].([]any)
	page, _ := out["page"].(map[string]any)
	cursor, _ := page["next_cursor"].(string)
	more, _ := page["has_more"].(bool)
	return data, cursor, more
}

// Read forward page by page: every event once, in order, and a cursor after the last page that
// finds nothing new rather than the same events again (UC-INS-14 check 6).
func TestTheFeedIsReadForwardEveryEventOnce(t *testing.T) {
	store := &feedStore{kept: keptEvents(3, fixed.Add(-time.Hour))}
	registry := feedRegistry(t, store, env.TenancyMulti)

	var seen []string
	cursor := ""
	for range 4 {
		in := usecase.Input{"limit": 2}
		if cursor != "" {
			in["cursor"] = cursor
		}
		out, err := registry.Invoke(t.Context(), ListPlatformEventsName, operator(), in)
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		data, next, more := pageOf(t, out)
		for _, row := range data {
			seen = append(seen, row.(map[string]any)["id"].(string))
		}
		if next == "" {
			t.Fatal("a page without a cursor to continue from")
		}
		cursor = next
		if !more {
			break
		}
	}
	if len(seen) != 3 || seen[0] >= seen[1] || seen[1] >= seen[2] {
		t.Errorf("read %v, want the three events once, in order", seen)
	}

	// Caught up, the cursor carries the read's moment, not the last event's: a platform that finds
	// nothing new for a long while is not told it missed something.
	position, _ := plainCursors{}.Decode(cursor)
	if position.Number != 30 || !position.At.Equal(fixed) {
		t.Errorf("the caught-up cursor is %+v", position)
	}
}

func TestACursorPastTheWindowIsGone(t *testing.T) {
	registry := feedRegistry(t, &feedStore{}, env.TenancyMulti)
	stale := plainCursors{}.Encode(adminrepo.PlatformPosition{
		Number: 10, At: fixed.Add(-PlatformEventsRetention - time.Minute),
	})
	_, err := registry.Invoke(t.Context(), ListPlatformEventsName, operator(), usecase.Input{"cursor": stale})
	var refusal *shared.Error
	if !errors.As(err, &refusal) || !errors.Is(err, shared.ErrGone) ||
		refusal.DetailCode != "admin.events_cursor_expired" || refusal.Params["retain_days"] != "30" {
		t.Errorf("a stale cursor answered %v", err)
	}

	fresh := plainCursors{}.Encode(adminrepo.PlatformPosition{Number: 10, At: fixed.Add(-PlatformEventsRetention + time.Minute)})
	if _, err := registry.Invoke(t.Context(), ListPlatformEventsName, operator(), usecase.Input{"cursor": fresh}); err != nil {
		t.Errorf("a cursor inside the window answered %v", err)
	}
}

func TestTheFeedIsTheOperatorsInMultiModeAlone(t *testing.T) {
	someoneElse := shared.ID("018f2a1b-0000-7000-8000-0000000000ff")
	noScope := operator()
	noScope.Scopes = nil
	cases := []struct {
		name     string
		registry *usecase.Registry
		actor    appshared.ActorContext
		want     string
	}{
		{"single mode", feedRegistry(t, &feedStore{}, env.TenancySingle), operator(), "admin.multi_mode_required"},
		{"outside the register", feedRegistry(t, &feedStore{}, env.TenancyMulti, someoneElse), operator(), "admin.operator_required"},
		{"without the scope", feedRegistry(t, &feedStore{}, env.TenancyMulti), noScope, "access.insufficient_scope"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.registry.Invoke(t.Context(), ListPlatformEventsName, c.actor, usecase.Input{})
			if refusalCode(err) != c.want || !errors.Is(err, shared.ErrForbidden) {
				t.Errorf("answered %v, want %s", err, c.want)
			}
		})
	}
}
