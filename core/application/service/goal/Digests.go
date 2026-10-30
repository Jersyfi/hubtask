// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The catalogue names and audit codes of a dashboard's digest.
const (
	ReadDashboardDigestName   = "ReadDashboardDigest"
	SetDashboardDigestName    = "SetDashboardDigest"
	DeleteDashboardDigestName = "DeleteDashboardDigest"

	DigestSetAction     audit.Action = "dashboard.digest_set"
	DigestRemovedAction audit.Action = "dashboard.digest_removed"
)

// The message codes of the digest's mail (i18n-l10n.md §3), a withheld variant without names for
// a recipient who lets no title travel.
const (
	digestSubject         = "email.dashboard_digest.subject"
	digestBody            = "email.dashboard_digest.body"
	digestWithheld        = ".withheld"
	digestLine            = "email.dashboard_digest.line"
	digestLineWithheld    = "email.dashboard_digest.line.withheld"
	digestLineUnavailable = "email.dashboard_digest.line.unavailable"
	digestNoNumbers       = "email.dashboard_digest.no_numbers"
)

// The scopes a digest is computed with: the reads the dashboard's figures take, and nothing else.
var digestScopes = []string{containersRead, itemsRead}

// Enqueuer queues a digest's job.
type Enqueuer interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// DigestJob is a digest's scheduled mail: one job per digest, rescheduling itself; the write that
// sets the digest seeds it.
func DigestJob(tenantID, digestID shared.ID, at time.Time) queue.Request {
	return queue.Request{
		Kind: queue.KindDashboardDigest, TenantID: tenantID, DedupeKey: digestID.String(), RunAt: at.UTC(),
		Payload: map[string]any{"digest_id": digestID.String()},
	}
}

// DigestOutput is a digest on the wire.
func DigestOutput(d domain.Digest) usecase.Output {
	var weekday any
	if d.Cadence == domain.CadenceWeekly {
		weekday = d.Weekday
	}
	var sent any
	if d.LastSentAt != nil {
		sent = *d.LastSentAt
	}
	return usecase.Output{
		"dashboard_id": d.DashboardID.String(), "cadence": string(d.Cadence), "weekday": weekday,
		"hour": d.Hour, "time_zone": d.TimeZone, "next_at": d.NextAt, "last_sent_at": sent,
	}
}

var digestDashboard = usecase.Field{Name: "dashboard_id", Kind: usecase.KindID, Required: true, Description: "The dashboard."}

// ReadDashboardDigest answers the caller's own digest of a dashboard.
type ReadDashboardDigest struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ReadDashboardDigest) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadDashboardDigestName, Permission: service.PermissionRead, ReadOnly: true,
		Summary: "The caller's own digest of a dashboard by mail: cadence, weekday, hour, zone and the next " +
			"moment it is sent. Not found where the caller has none.",
		SideEffects: "None. Reads only.", TokenScope: containersRead,
		Input: []usecase.Field{digestDashboard},
		Audit: usecase.AuditDeclaration{Action: DashboardReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("dashboard_id")
			if err != nil {
				return nil, err
			}
			if _, err := s.dashboard(ctx, actor, id); err != nil {
				return nil, err
			}
			var d domain.Digest
			if err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				d, err = s.Store.Digest(ctx, id, actor.AccountID)
				return err
			}); err != nil {
				return nil, err
			}
			return DigestOutput(d), nil
		}),
	}, dashboardTarget)
}

// SetDashboardDigest subscribes the caller to a dashboard they see, by mail, replacing their
// digest of it. Only oneself: a digest is computed as its subscriber, so nobody is sent numbers
// somebody else chose for them.
type SetDashboardDigest struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h SetDashboardDigest) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetDashboardDigestName, Permission: service.PermissionRead,
		Summary: "Sends the caller a dashboard they see by mail, daily or on one weekday at an hour of their " +
			"zone: a link and the totals of its NUMBER widgets, computed as the caller when it is sent. " +
			"Replaces the caller's digest of the dashboard.",
		SideEffects: "Writes the digest, schedules its mail and writes an audit entry.", TokenScope: containersWrite,
		Input: []usecase.Field{
			digestDashboard,
			{Name: "cadence", Kind: usecase.KindString, Required: true, Enum: []string{"DAILY", "WEEKLY"}, Description: "DAILY, or WEEKLY on the weekday."},
			{Name: "weekday", Kind: usecase.KindInt, Description: "WEEKLY's day, ISO: 1 Monday … 7 Sunday."},
			{Name: "hour", Kind: usecase.KindInt, Required: true, Description: "The hour, 0 to 23, in the zone."},
			{Name: "time_zone", Kind: usecase.KindString, Description: "An IANA zone; the caller's own without."},
		},
		Audit: usecase.AuditDeclaration{Action: DigestSetAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			id, err := in.ID("dashboard_id")
			if err != nil {
				return nil, err
			}
			dashboard, err := s.dashboard(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			weekday, _ := intOf(in["weekday"])
			hour, ok := intOf(in["hour"])
			if !ok {
				return nil, invalid("/hour", "dashboards.digest_hour_invalid")
			}
			zone := in.String("time_zone")
			if zone == "" {
				zone = actor.TimeZone
			}
			d, err := domain.CheckDigest(domain.Digest{
				ID: s.IDs.NewID(), DashboardID: dashboard.ID, AccountID: actor.AccountID,
				Cadence: domain.Cadence(in.String("cadence")), Weekday: weekday, Hour: hour, TimeZone: zone,
			})
			if err != nil {
				return nil, err
			}
			now := s.Clock.Now()
			d.NextAt = d.Next(now)
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				kept, err := s.Store.PutDigest(ctx, d, now)
				if err != nil {
					return err
				}
				d.ID = kept
				if s.Jobs != nil {
					if _, err := s.Jobs.Enqueue(ctx, DigestJob(actor.TenantID, d.ID, d.NextAt)); err != nil {
						return err
					}
				}
				return s.record(ctx, actor, DigestSetAction, dashboardTarget, dashboard.ID, now)
			}); err != nil {
				return nil, err
			}
			return DigestOutput(d), nil
		}),
	}, dashboardTarget)
}

// DeleteDashboardDigest ends the caller's digest of a dashboard.
type DeleteDashboardDigest struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h DeleteDashboardDigest) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DeleteDashboardDigestName, Permission: service.PermissionRead, Destructive: true,
		Summary:     "Ends the caller's digest of a dashboard; its scheduled mail finds nothing and stops.",
		SideEffects: "Deletes the digest and writes an audit entry.", TokenScope: containersWrite,
		Input: []usecase.Field{digestDashboard},
		Audit: usecase.AuditDeclaration{Action: DigestRemovedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			s := h.Steward
			if err := person(actor); err != nil {
				return nil, err
			}
			if err := actor.RequireScope(containersWrite); err != nil {
				return nil, err
			}
			id, err := in.ID("dashboard_id")
			if err != nil {
				return nil, err
			}
			now := s.Clock.Now()
			var gone bool
			if err := s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				if gone, err = s.Store.DeleteDigest(ctx, id, actor.AccountID); err != nil || !gone {
					return err
				}
				return s.record(ctx, actor, DigestRemovedAction, dashboardTarget, id, now)
			}); err != nil {
				return nil, err
			}
			if !gone {
				return nil, notFound("dashboards.digest_not_found")
			}
			return usecase.Output{"deleted": true}, nil
		}),
	}, dashboardTarget)
}

// DigestAccounts answers the subscriber's account.
type DigestAccounts interface {
	Find(ctx context.Context, id shared.ID) (identity.Account, error)
}

// DigestPreferences answers what the subscriber said about titles in mail - REMINDER's email
// preference, the category of a moment somebody set for themselves.
type DigestPreferences interface {
	Find(ctx context.Context, accountID shared.ID, category notification.Category, channel notification.Channel) (notification.Preference, error)
}

// DigestAddresses answers the workspace's address, so the mail carries a link.
type DigestAddresses interface {
	Of(ctx context.Context) (string, error)
}

// DashboardDigests sends digests when they are owed. Not a use case: nobody asks for it, the
// schedule does, and it acts as the subscriber - evaluated now, as a calendar feed is - so a
// person who lost a dashboard or a hub is sent no more of it.
type DashboardDigests struct {
	Steward     Steward
	Accounts    DigestAccounts
	Preferences DigestPreferences
	Addresses   DigestAddresses
	Mail        mail.Sender
	Renderer    i18n.Renderer
	// Decimals writes a total's fraction with the subscriber's separator; nil keeps the point.
	Decimals i18n.DecimalSeparators
	// Background runs the job's own transactions, outside any request.
	Background     persistence.UnitOfWork
	FallbackLocale string
}

// digestRead is what a run needs before it computes.
type digestRead struct {
	digest  domain.Digest
	account identity.Account
	titles  bool
	base    string
}

// RunScheduled sends a digest that is owed and answers the next moment; zero when the digest, its
// subscriber or their address is gone, which ends the job.
func (r DashboardDigests) RunScheduled(ctx context.Context, tenantID, digestID shared.ID) (time.Time, error) {
	scope := persistence.Scope{TenantID: tenantID}
	now := r.Steward.Clock.Now()
	var read digestRead
	err := r.Background.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		if read.digest, err = r.Steward.Store.DigestByID(ctx, digestID); err != nil {
			return err
		}
		if read.account, err = r.Accounts.Find(ctx, read.digest.AccountID); err != nil {
			return err
		}
		read.titles = true
		if r.Preferences != nil {
			p, err := r.Preferences.Find(ctx, read.account.ID, notification.CategoryReminder, notification.ChannelEmail)
			switch {
			case err == nil:
				read.titles = p.IncludeTitle
			case !errors.Is(err, shared.ErrNotFound):
				return err
			}
		}
		if r.Addresses != nil {
			read.base, err = r.Addresses.Of(ctx)
		}
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	d, account := read.digest, read.account
	if account.Verify() != nil || account.Email == "" {
		return time.Time{}, nil
	}
	if !d.Due(now) {
		return d.NextAt, nil
	}
	next := d.Next(now)
	actor := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: account.ID, AccountName: account.DisplayName,
		Locale: account.Locale, TimeZone: account.TimeZone, WeekStart: account.WeekStart, Scopes: digestScopes,
	}
	message, ok, err := r.compose(ctx, actor, d, read)
	if err != nil {
		return time.Time{}, err
	}
	var sent *time.Time
	if ok {
		if err := r.Mail.Send(ctx, message); err != nil {
			return time.Time{}, err
		}
		sent = &now
	}
	return next, r.Background.Within(ctx, scope, func(ctx context.Context) error {
		return r.Steward.Store.DigestSent(ctx, d.ID, sent, next)
	})
}

// compose renders the mail in the subscriber's language; false when the subscriber no longer sees
// the dashboard, which skips this one quietly, as a feed narrows.
func (r DashboardDigests) compose(ctx context.Context, actor appshared.ActorContext, d domain.Digest, read digestRead) (mail.Message, bool, error) {
	dashboard, err := r.Steward.dashboard(ctx, actor, d.DashboardID)
	if errors.Is(err, shared.ErrNotFound) || errors.Is(err, shared.ErrForbidden) {
		return mail.Message{}, false, nil
	}
	if err != nil {
		return mail.Message{}, false, err
	}
	figures, err := r.Steward.figures(ctx, actor, dashboard)
	if err != nil {
		return mail.Message{}, false, err
	}
	locale := read.account.Locale
	if locale == "" {
		locale = r.FallbackLocale
	}
	separator := "."
	if r.Decimals != nil {
		separator = r.Decimals.DecimalSeparatorOf(locale)
	}
	lines := digestLines(dashboard, figures, read.titles, separator, func(code string, params map[string]string) string {
		return r.Renderer.Render(locale, code, params)
	})
	link := ""
	if read.base != "" {
		link = strings.TrimRight(read.base, "/") + "/dashboards/" + url.PathEscape(dashboard.ID.String())
	}
	params := map[string]string{"figures": lines, "link": link}
	subject, body := digestSubject, digestBody
	if read.titles {
		params["name"] = notification.TrimSubject(dashboard.Name)
	} else {
		subject, body = subject+digestWithheld, body+digestWithheld
	}
	return mail.Message{
		To:      read.account.Email,
		Subject: r.Renderer.Render(locale, subject, params),
		Body:    r.Renderer.Render(locale, body, params),
	}, true, nil
}

// digestLines is one line per NUMBER widget in reading order: its title and total, or that the
// subscriber cannot compute it; numbered instead of titled where no title may travel. A total is
// exact, its fraction after the subscriber's separator and its digits ungrouped (i18n-l10n.md §3).
func digestLines(d domain.Dashboard, figures usecase.Output, titles bool, separator string, render func(string, map[string]string) string) string {
	byID := map[string]usecase.Output{}
	if widgets, ok := figures["widgets"].([]usecase.Output); ok {
		for _, w := range widgets {
			id, _ := w["id"].(string)
			byID[id] = w
		}
	}
	ordered := append([]domain.Widget(nil), d.Widgets...)
	sortReading(ordered)
	var out []string
	n := 0
	for _, w := range ordered {
		if w.Chart != domain.ChartNumber || len(out) >= domain.MaxDigestLines {
			continue
		}
		n++
		params := map[string]string{"title": "#" + itoa(n), "position": itoa(n)}
		if titles {
			params["title"] = w.Title
		}
		figure := byID[w.ID.String()]
		total, has := "", false
		if figure != nil && figure["error"] == nil {
			total, has = domain.Total(rowsOf(figure["data"]))
		}
		params["total"] = strings.Replace(total, ".", separator, 1)
		switch {
		case !has:
			delete(params, "total")
			out = append(out, render(digestLineUnavailable, params))
		case titles:
			out = append(out, render(digestLine, params))
		default:
			delete(params, "title")
			out = append(out, render(digestLineWithheld, params))
		}
	}
	if len(out) == 0 {
		return render(digestNoNumbers, nil)
	}
	return strings.Join(out, "\n")
}

func rowsOf(v any) []map[string]any {
	switch rows := v.(type) {
	case []usecase.Output:
		out := make([]map[string]any, len(rows))
		for i, r := range rows {
			out[i] = r
		}
		return out
	case []map[string]any:
		return rows
	case []any:
		out := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			if m, ok := r.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// sortReading orders widgets as the grid is read: by row, then column.
func sortReading(ws []domain.Widget) {
	for i := 1; i < len(ws); i++ {
		for j := i; j > 0 && (ws[j].Y < ws[j-1].Y || (ws[j].Y == ws[j-1].Y && ws[j].X < ws[j-1].X)); j-- {
			ws[j], ws[j-1] = ws[j-1], ws[j]
		}
	}
}
