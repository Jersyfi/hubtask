// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// SignatureWindow is how far a signed delivery's moment may be from the server's: older ones are
// refused, and the digests of the window's deliveries are kept so that one replayed inside it is
// refused too (research itsm-cmdb-ai C3).
const SignatureWindow = 5 * time.Minute

// The use cases an alert's entry is opened, closed and reopened with, through the registry.
const (
	createEntryUseCase   = "CreateWorkItem"
	completeEntryUseCase = "CompleteWorkItem"
	reopenEntryUseCase   = "ReopenWorkItem"
)

// AlertDelivery is one post to an alert intake, as the intake adapter received it. Either the
// bearer secret is presented, or the delivery is signed: an HMAC-SHA-256 of the moment, a dot and
// the body, keyed by the secret, in hex.
type AlertDelivery struct {
	TenantID    shared.ID
	ConnectorID shared.ID
	// Bearer is the secret as presented: a bearer token, or a basic password for a program that
	// knows no other way.
	Bearer    string
	Signature string
	// Timestamp is the signed moment in Unix seconds.
	Timestamp string
	Body      []byte
}

// AlertParser translates a monitoring program's delivery into signals; the format is the
// adapter's, the kind the connector's.
type AlertParser func(kind domain.ConnectorKind, body []byte) ([]domain.AlertSignal, error)

// AlertReceipt is what a delivery did.
type AlertReceipt struct {
	Received  int
	Opened    int
	Resolved  int
	Unchanged int
	// EntriesFailed counts entries that could not be opened, closed or reopened; the alert is
	// kept regardless.
	EntriesFailed int
}

// intakeNotFound is every refusal before the delivery is trusted: an unknown connector, one
// switched off, an intake without a secret, a wrong secret, a stale or replayed signature. One
// answer, so that nothing tells somebody guessing which check failed (T-21).
func intakeNotFound() error { return shared.ErrNotFound.WithDetail("cmdb.intake_not_found") }

// DeliverAlerts authenticates a delivery by the intake's sealed secret and applies its alerts:
// each problem once by its key, bound to the CI its hosts name, with an entry opened in the
// intake's collection while it fires and completed when it resolves (research itsm-cmdb-ai C10).
func (s Connectors) DeliverAlerts(ctx context.Context, d AlertDelivery, parse AlertParser) (AlertReceipt, error) {
	c, actor, err := s.intake(ctx, d, false)
	if err != nil {
		return AlertReceipt{}, err
	}
	scope := appshared.ActorContext{TenantID: d.TenantID}.PersistenceScope()
	now := s.Inventory.Clock.Now()
	signals, err := parse(c.Kind, d.Body)
	if err != nil {
		return AlertReceipt{}, err
	}
	if len(signals) > domain.MaxAlertsPerDelivery {
		return AlertReceipt{}, invalid("/alerts", "cmdb.alerts_too_many", "maximum", itoa(domain.MaxAlertsPerDelivery))
	}
	checked := make([]domain.AlertSignal, 0, len(signals))
	for _, sig := range signals {
		sig, err := domain.CheckSignal(sig)
		if err != nil {
			return AlertReceipt{}, err
		}
		checked = append(checked, sig)
	}
	var receipt AlertReceipt
	for _, sig := range checked {
		if err := s.receive(ctx, actor, c, sig, now, &receipt); err != nil {
			return receipt, err
		}
	}
	err = s.Inventory.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		return s.Inventory.record(ctx, actor, AlertsReceivedAction, connectorTarget, c.ID,
			audit.Change{Field: "received", Classification: audit.Open, To: receipt.Received},
			audit.Change{Field: "opened", Classification: audit.Open, To: receipt.Opened},
			audit.Change{Field: "resolved", Classification: audit.Open, To: receipt.Resolved},
			audit.Change{Field: "entries_failed", Classification: audit.Open, To: receipt.EntriesFailed})
	})
	return receipt, err
}

// intake answers the push intake a delivery names, once it is trusted: of the asked shape (an
// inventory intake or an alert one), switched on, with a secret, and presented that secret or a
// fresh signature by it. Every refusal is the same intakeNotFound.
func (s Connectors) intake(ctx context.Context, d AlertDelivery, inventory bool) (domain.Connector, appshared.ActorContext, error) {
	scope := appshared.ActorContext{TenantID: d.TenantID}.PersistenceScope()
	var c domain.Connector
	if err := s.Inventory.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		c, err = s.Store.FindConnector(ctx, d.ConnectorID, false)
		return err
	}); err != nil || !c.Spec().Push || c.Spec().Inventory != inventory || !c.Enabled || !c.HasCredential {
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return domain.Connector{}, appshared.ActorContext{}, err
		}
		return domain.Connector{}, appshared.ActorContext{}, intakeNotFound()
	}
	actor := s.runAs(d.TenantID, c)
	key, err := s.credential(ctx, actor, c)
	if err != nil {
		return domain.Connector{}, appshared.ActorContext{}, intakeNotFound()
	}
	now := s.Inventory.Clock.Now()
	digest, ok := verifyDelivery(d, key.Reveal(), now)
	if !ok {
		return domain.Connector{}, appshared.ActorContext{}, intakeNotFound()
	}
	if digest != nil {
		fresh := false
		if err := s.Inventory.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
			var err error
			fresh, err = s.Store.SeeDigest(ctx, c.ID, digest, now, now.Add(-2*SignatureWindow))
			return err
		}); err != nil {
			return domain.Connector{}, appshared.ActorContext{}, err
		}
		if !fresh {
			return domain.Connector{}, appshared.ActorContext{}, intakeNotFound()
		}
	}
	return c, actor, nil
}

// verifyDelivery answers whether the delivery carries the secret, and for a signed one the digest
// a replay is recognised by.
func verifyDelivery(d AlertDelivery, key string, now time.Time) ([]byte, bool) {
	if d.Bearer != "" {
		return nil, subtle.ConstantTimeCompare([]byte(d.Bearer), []byte(key)) == 1
	}
	if d.Signature == "" || d.Timestamp == "" {
		return nil, false
	}
	seconds, err := strconv.ParseInt(d.Timestamp, 10, 64)
	if err != nil {
		return nil, false
	}
	at := time.Unix(seconds, 0)
	if at.Before(now.Add(-SignatureWindow)) || at.After(now.Add(SignatureWindow)) {
		return nil, false
	}
	presented, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(d.Signature), "sha256="))
	if err != nil {
		return nil, false
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(d.Timestamp + "."))
	mac.Write(d.Body)
	if !hmac.Equal(presented, mac.Sum(nil)) {
		return nil, false
	}
	sum := sha256.Sum256(presented)
	return sum[:], true
}

// Sign is the signature a monitoring program sends with a delivery, for its documentation and
// its tests.
func Sign(key, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func (s Connectors) receive(ctx context.Context, actor appshared.ActorContext, c domain.Connector, sig domain.AlertSignal, now time.Time, r *AlertReceipt) error {
	r.Received++
	var alert domain.Alert
	var change domain.AlertChange
	err := s.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		existing, err := s.Store.FindAlert(ctx, c.ID, sig.Key)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		alert, change = domain.Receive(existing, sig, now)
		if change == domain.AlertUnchanged {
			return nil
		}
		if alert.ID.IsZero() {
			alert.ID, alert.TenantID, alert.HubID, alert.Connector = s.Inventory.IDs.NewID(), actor.TenantID, c.HubID, c.ID
		}
		if alert.CI.IsZero() {
			if alert.CI, err = s.affected(ctx, c.HubID, sig); err != nil {
				return err
			}
		}
		return s.Store.PutAlert(ctx, alert)
	})
	if err != nil {
		return err
	}
	switch change {
	case domain.AlertUnchanged:
		r.Unchanged++
		return nil
	case domain.AlertOpened, domain.AlertReopened:
		r.Opened++
	case domain.AlertClosed:
		r.Resolved++
	}
	return s.entry(ctx, actor, c, alert, change, r)
}

// affected answers the one CI an alert's hosts name, by host name, address or CI name; none when
// no lookup names exactly one.
func (s Connectors) affected(ctx context.Context, hubID shared.ID, sig domain.AlertSignal) (shared.ID, error) {
	for _, pairs := range domain.AlertLookups(sig) {
		for name, value := range pairs {
			rows, err := s.Inventory.Store.FindIdentifiers(ctx, hubID, map[string]string{name: value})
			if err != nil {
				return "", err
			}
			found := map[shared.ID]bool{}
			for _, row := range rows {
				if row.Name == name && row.Value == value {
					found[row.CI] = true
				}
			}
			if len(found) == 1 {
				for id := range found {
					return id, nil
				}
			}
		}
	}
	return "", nil
}

// entry opens, closes or reopens the alert's entry through the registry, as the intake's person.
func (s Connectors) entry(ctx context.Context, actor appshared.ActorContext, c domain.Connector, a domain.Alert, change domain.AlertChange, r *AlertReceipt) error {
	if s.Entries == nil || c.Collection.IsZero() {
		return nil
	}
	var err error
	switch {
	case a.Entry.IsZero() && (change == domain.AlertOpened || change == domain.AlertReopened):
		title := a.Title
		if title == "" {
			title = a.Key
		}
		in := usecase.Input{"type": "TASK", "title": title, "collection_id": c.Collection.String()}
		if !a.CI.IsZero() {
			in["notes"] = "![](hubtask:ci/" + a.CI.String() + ")"
		}
		var out usecase.Output
		if out, err = s.Entries.Invoke(ctx, createEntryUseCase, actor, in); err == nil {
			if id, perr := shared.ParseID(out.String("id")); perr == nil {
				a.Entry = id
				err = s.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
					return s.Store.PutAlert(ctx, a)
				})
				if err != nil {
					return err
				}
			}
		}
	case !a.Entry.IsZero() && change == domain.AlertClosed:
		_, err = s.Entries.Invoke(ctx, completeEntryUseCase, actor, usecase.Input{"item_id": a.Entry.String()})
	case !a.Entry.IsZero() && change == domain.AlertReopened:
		_, err = s.Entries.Invoke(ctx, reopenEntryUseCase, actor, usecase.Input{"item_id": a.Entry.String()})
	}
	if err != nil {
		// The alert is kept; the entry is the person's to fix, and the receipt counts it.
		r.EntriesFailed++
	}
	return nil
}
