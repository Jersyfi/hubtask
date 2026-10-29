// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/cmdb"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	connectorport "github.com/Jersyfi/hubtask/core/port/connector"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The use case names.
const (
	ListCiConnectorsName         = "ListCiConnectors"
	DefineCiConnectorName        = "DefineCiConnector"
	SetCiConnectorCredentialName = "SetCiConnectorCredential"
	RunCiConnectorName           = "RunCiConnector"
	ListCiAlertsName             = "ListCiAlerts"
)

const (
	connectorTarget = "cmdb_connector"
	alertTarget     = "cmdb_alert"
)

// The stable audit codes of the connectors (audit.md §2). A credential never reaches the trail,
// nor an alert's title (rule 10).
const (
	ConnectorDefinedAction    audit.Action = "cmdb.connector_defined"
	ConnectorCredentialAction audit.Action = "cmdb.connector_credential_set"
	ConnectorRanAction        audit.Action = "cmdb.connector_ran"
	ConnectorReadAction       audit.Action = "cmdb.connector_read"
	AlertsReceivedAction      audit.Action = "cmdb.alerts_received"
)

// MaxPagesPerRun bounds one run; a source with more changes continues at the next run, from the
// cursor the last page reached.
const MaxPagesPerRun = 50

// intakeSecretBytes is the entropy of an alert intake's secret, a personal access token's.
const intakeSecretBytes = 32

// CredentialPurpose binds a sealed credential to its connector, so that a ciphertext moved to
// another row does not open (security.md §8.1).
func CredentialPurpose(id shared.ID) crypto.Purpose {
	return crypto.Purpose("cmdb.connector:" + id.String())
}

// Enqueuer is the slice of the job queue a connector's schedule needs.
type Enqueuer interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// Invoker runs a use case by name through the registry - the one door - for the entry an alert
// opens or closes.
type Invoker interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// Connectors is what the connector use cases share: the inventory they write through, their
// store, the pullers, the sealing and the schedule.
type Connectors struct {
	Inventory Inventory
	Store     repository.Connectors
	Pullers   connectorport.Puller
	Encryptor crypto.Encryptor
	Entropy   clock.Entropy
	Jobs      Enqueuer
	// Entries opens and closes an alert's entry; nil records alerts only.
	Entries Invoker
}

// PullJob is a connector's scheduled pull: one job per connector, rescheduling itself.
func PullJob(tenantID, connectorID shared.ID, at time.Time) queue.Request {
	return queue.Request{
		Kind: queue.KindCmdbConnectorPull, TenantID: tenantID, DedupeKey: connectorID.String(), RunAt: at.UTC(),
		Payload: map[string]any{"connector_id": connectorID.String()},
	}
}

func (s Connectors) schedule(ctx context.Context, tenantID shared.ID, c domain.Connector) error {
	if s.Jobs == nil || c.NextRunAt == nil {
		return nil
	}
	_, err := s.Jobs.Enqueue(ctx, PullJob(tenantID, c.ID, *c.NextRunAt))
	return err
}

// ConnectorOutput is a connector as every door returns it (schema CiConnector): never its
// credential and never its cursor, which may carry a source's own token.
func ConnectorOutput(c domain.Connector) usecase.Output {
	var next, last any
	if c.NextRunAt != nil {
		next = *c.NextRunAt
	}
	if c.LastRunAt != nil {
		last = *c.LastRunAt
	}
	settings := map[string]any{}
	for k, v := range c.Settings {
		settings[k] = v
	}
	return usecase.Output{
		"id": c.ID.String(), "hub_id": c.HubID.String(), "key": c.Key, "kind": string(c.Kind), "source": c.Source,
		"push": c.Spec().Push, "base_url": c.BaseURL, "settings": settings,
		"allowed_hosts": append([]string{}, c.AllowedHosts...), "hosts": c.Hosts(),
		"every_minutes": c.Every, "enabled": c.Enabled, "collection_id": idOrNil(c.Collection),
		"run_as": idOrNil(c.RunAs), "has_credential": c.HasCredential, "next_run_at": next, "last_run_at": last,
		"last_run_id": idOrNil(c.LastRun), "last_outcome": string(c.LastOutcome), "last_error": c.LastError,
		"failures": c.Failures, "created_at": c.CreatedAt, "updated_at": c.UpdatedAt, "version": c.Version,
	}
}

// AlertOutput is an alert as every door returns it (schema CiAlert).
func AlertOutput(a domain.Alert) usecase.Output {
	var resolved any
	if a.ResolvedAt != nil {
		resolved = *a.ResolvedAt
	}
	return usecase.Output{
		"id": a.ID.String(), "hub_id": a.HubID.String(), "connector_id": a.Connector.String(), "key": a.Key,
		"status": string(a.Status), "severity": a.Severity, "title": a.Title, "ci_id": idOrNil(a.CI),
		"entry_id": idOrNil(a.Entry), "first_at": a.FirstAt, "last_at": a.LastAt, "resolved_at": resolved,
		"count": a.Count,
	}
}

// connector reads one outside a write, for the permission question its hub decides.
func (s Connectors) connector(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Connector, error) {
	var c domain.Connector
	err := s.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		c, err = s.Store.FindConnector(ctx, id, false)
		return err
	})
	return c, err
}

// ListCiConnectors answers a hub's connectors.
type ListCiConnectors struct{ Connectors Connectors }

// Descriptor is the catalogue entry.
func (h ListCiConnectors) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiConnectorsName, Permission: service.PermissionRead,
		Summary: "Lists a hub's connectors: the programs its configuration items are pulled from (GLPI, NetBox, " +
			"Intune, Entra ID, Hetzner Cloud, OpenStack) and the monitoring programs that post alerts to it " +
			"(Alertmanager, Zabbix, Checkmk), each with the source it writes as, the hosts it may reach, its " +
			"period, whether a credential is set (never the credential) and how its last run ended.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."}},
		Audit: usecase.AuditDeclaration{Action: ConnectorReadAction, TargetType: connectorTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			s := h.Connectors
			if err := s.Inventory.authorizeHub(ctx, actor, hub, false, ConnectorReadAction, connectorTarget, hub); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = s.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				list, err := s.Store.ListConnectors(ctx, hub)
				for _, c := range list {
					rows = append(rows, ConnectorOutput(c))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	})
}

// DefineCiConnector adds or changes a hub's connector.
type DefineCiConnector struct{ Connectors Connectors }

// Descriptor is the catalogue entry.
func (h DefineCiConnector) Descriptor() usecase.Descriptor {
	kinds := make([]string, 0, len(domain.ConnectorSpecs()))
	for _, s := range domain.ConnectorSpecs() {
		kinds = append(kinds, string(s.Kind))
	}
	return descriptor(usecase.Descriptor{
		Name: DefineCiConnectorName, Permission: service.PermissionStructure,
		Summary: "Adds or changes a connector in a hub: its kind, the workspace's connector source it writes as " +
			"(whose rank and classes decide what it may change), the address it pulls from over https, the " +
			"kind's settings (a client's identifier, a directory, an application credential), further hosts it " +
			"may reach, a period of 15 minutes to a week or none, whether it is on, and - for an alert intake - " +
			"the collection a firing alert opens an entry in. The credential is set on its own and never " +
			"answered. A connector is never deleted; it is switched off.",
		SideEffects: "Writes the connector, schedules its next pull and writes an audit entry.", TokenScope: configWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub whose inventory it feeds."},
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "Small letters, digits and underscores, unique in the workspace."},
			{Name: "kind", Kind: usecase.KindString, Required: true, Enum: kinds, Description: "The program; fixed once made."},
			{Name: "source", Kind: usecase.KindString, Required: true, Description: "The key of a source of kind CONNECTOR it writes as."},
			{Name: "base_url", Kind: usecase.KindString, Description: "The https address it pulls from; Graph and Hetzner have one of their own."},
			{Name: "settings", Kind: usecase.KindObject, Description: "The kind's settings by key: GLPI client_id; Intune and Entra tenant and client_id; OpenStack application_credential_id and region."},
			{Name: "allowed_hosts", Kind: usecase.KindList, Description: "Up to 10 further host names it may reach, such as an OpenStack compute endpoint's."},
			{Name: "every_minutes", Kind: usecase.KindInt, Description: "The period of a pull, 15 to 10080; 0 or absent pulls only when asked."},
			{Name: "enabled", Kind: usecase.KindBool, Description: "Whether it runs and takes alerts; true when absent."},
			{Name: "collection_id", Kind: usecase.KindID, Description: "An alert intake's collection in the hub, where a firing alert opens an entry."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read, for a change."},
		},
		Audit: usecase.AuditDeclaration{Action: ConnectorDefinedAction, TargetType: connectorTarget, Severity: audit.SeverityNotice, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			return h.Connectors.define(ctx, actor, in)
		}),
	})
}

func (s Connectors) define(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	hub, err := in.ID("hub_id")
	if err != nil {
		return nil, err
	}
	var collection shared.ID
	if in.Present("collection_id") && in.String("collection_id") != "" {
		if collection, err = in.ID("collection_id"); err != nil {
			return nil, err
		}
	}
	if err := s.Inventory.authorizeConfig(ctx, actor, true, ConnectorDefinedAction, connectorTarget); err != nil {
		return nil, err
	}
	enabled := true
	if in.Present("enabled") {
		enabled = in.Bool("enabled")
	}
	var out usecase.Output
	err = s.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := s.Inventory.hub(ctx, hub); err != nil {
			return err
		}
		if !collection.IsZero() {
			c, err := s.Inventory.Containers.Find(ctx, collection)
			if err != nil || c.Type != work.ContainerCollection || c.ParentID != hub || c.DeletedAt != nil {
				return invalid("/collection_id", "cmdb.connector_collection_not_in_hub")
			}
		}
		reg, err := s.Inventory.registry(ctx)
		if err != nil {
			return err
		}
		source, ok := reg.sources[in.String("source")]
		if !ok {
			return shared.ErrNotFound.WithDetail("cmdb.source_not_found").WithParams(map[string]string{"key": in.String("source")})
		}
		list, err := s.Store.ListConnectors(ctx, "")
		if err != nil {
			return err
		}
		var existing *domain.Connector
		expected := 0
		for i := range list {
			if list[i].Key == in.String("key") {
				existing, expected = &list[i], list[i].Version
			}
		}
		if existing != nil && in.Present("expected_version") && in.Int("expected_version") != existing.Version {
			return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
		}
		if existing == nil {
			n, err := s.Store.CountConnectors(ctx, hub)
			if err != nil {
				return err
			}
			if n >= domain.MaxConnectorsPerHub {
				return invalid("/key", "cmdb.connectors_too_many", "maximum", itoa(domain.MaxConnectorsPerHub))
			}
		}
		c, err := domain.NewConnector(domain.ConnectorInput{
			ID: s.Inventory.IDs.NewID(), TenantID: actor.TenantID, HubID: hub, Key: in.String("key"),
			Kind: domain.ConnectorKind(in.String("kind")), Source: source, BaseURL: in.String("base_url"),
			Settings: textMap(in["settings"]), AllowedHosts: texts(in["allowed_hosts"]), Every: in.Int("every_minutes"),
			Enabled: enabled, Collection: collection, By: actor.AccountID, Now: s.Inventory.Clock.Now(),
		}, existing)
		if err != nil {
			return err
		}
		if err := s.Store.PutConnector(ctx, c, expected); err != nil {
			return err
		}
		c.Version = expected + 1
		if err := s.schedule(ctx, actor.TenantID, c); err != nil {
			return err
		}
		out = ConnectorOutput(c)
		return s.Inventory.record(ctx, actor, ConnectorDefinedAction, connectorTarget, c.ID,
			audit.Change{Field: "key", Classification: audit.Open, To: c.Key},
			audit.Change{Field: "kind", Classification: audit.Open, To: string(c.Kind)},
			audit.Change{Field: "source", Classification: audit.Open, To: c.Source},
			audit.Change{Field: "enabled", Classification: audit.Open, To: c.Enabled})
	})
	return out, err
}

// SetCiConnectorCredential seals a connector's credential.
type SetCiConnectorCredential struct{ Connectors Connectors }

// Descriptor is the catalogue entry.
func (h SetCiConnectorCredential) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetCiConnectorCredentialName, Permission: service.PermissionStructure,
		Summary: "Sets a connector's credential, sealed at rest and never answered again: for a pull, the secret " +
			"the program gave (a GLPI or Graph client secret, a NetBox or Hetzner token, an OpenStack application " +
			"credential's secret). For an alert intake Hubtask draws the secret itself and answers it once: the " +
			"monitoring program presents it as a bearer token or signs each delivery with it. Setting it again " +
			"replaces the old one at once.",
		SideEffects: "Seals and writes the credential and an audit entry; for an alert intake answers a new secret once.",
		TokenScope:  configWrite,
		Input: []usecase.Field{
			{Name: "connector_id", Kind: usecase.KindID, Required: true, Description: "The connector."},
			{Name: "credential", Kind: usecase.KindString, Description: "A pull's secret; absent for an alert intake, whose secret is drawn."},
		},
		Audit: usecase.AuditDeclaration{Action: ConnectorCredentialAction, TargetType: connectorTarget, Severity: audit.SeverityNotice, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("connector_id")
			if err != nil {
				return nil, err
			}
			return h.Connectors.setCredential(ctx, actor, id, in.String("credential"))
		}),
	})
}

func (s Connectors) setCredential(ctx context.Context, actor appshared.ActorContext, id shared.ID, given string) (usecase.Output, error) {
	if err := s.Inventory.authorizeConfig(ctx, actor, true, ConnectorCredentialAction, connectorTarget); err != nil {
		return nil, err
	}
	c, err := s.connector(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	value := given
	drawn := c.Spec().Push
	switch {
	case drawn && given != "":
		return nil, invalid("/credential", "cmdb.connector_credential_drawn")
	case drawn:
		material, err := s.Entropy.Bytes(intakeSecretBytes)
		if err != nil {
			return nil, shared.ErrUnavailable.WithDetail("cmdb.connector_secret_undrawable").WithCause(err)
		}
		value = base64.RawURLEncoding.EncodeToString(material)
	case given == "" || len(given) > 4096:
		return nil, invalid("/credential", "cmdb.connector_credential_invalid")
	}
	sealed, err := s.Encryptor.Seal(ctx, secret.New(value), CredentialPurpose(c.ID))
	if err != nil {
		return nil, err
	}
	now := s.Inventory.Clock.Now()
	err = s.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := s.Store.SetCredential(ctx, c.ID, sealed.Ciphertext, sealed.KeyID, now); err != nil {
			return err
		}
		return s.Inventory.record(ctx, actor, ConnectorCredentialAction, connectorTarget, c.ID,
			audit.Change{Field: "credential", Classification: audit.Secret})
	})
	if err != nil {
		return nil, err
	}
	c.HasCredential, c.Version, c.UpdatedAt = true, c.Version+1, now
	out := ConnectorOutput(c)
	if drawn {
		// The one place the intake's secret is ever answered.
		out["secret"] = value
	}
	return out, nil
}

// RunCiConnector pulls a connector's source now.
type RunCiConnector struct{ Connectors Connectors }

// Descriptor is the catalogue entry.
func (h RunCiConnector) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RunCiConnectorName, Permission: service.PermissionWriteItems,
		Summary: "Pulls a connector's source now: page by page from where its last run ended - or from the " +
			"beginning when a full walk is asked - each page through the one reconciliation as the connector's " +
			"source, up to 50 pages, the rest at its next run. A dry run fetches the first page and answers what " +
			"the reconciliation would do with it, writing nothing. A failure is answered by its message code and " +
			"kept on the connector; the cursor stays, so nothing is skipped.",
		SideEffects: "Calls the source; writes what the reconciliation writes, the connector's run record and audit entries; nothing on a dry run.",
		TokenScope:  cisWrite,
		Input: []usecase.Field{
			{Name: "connector_id", Kind: usecase.KindID, Required: true, Description: "The connector."},
			{Name: "full", Kind: usecase.KindBool, Description: "Walk the whole source again rather than from the last cursor."},
			dryRunField,
		},
		Audit: usecase.AuditDeclaration{Action: ConnectorRanAction, TargetType: connectorTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("connector_id")
			if err != nil {
				return nil, err
			}
			s := h.Connectors
			c, err := s.connector(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if c.Spec().Push {
				return nil, invalid("/connector_id", "cmdb.connector_is_intake")
			}
			if err := s.Inventory.authorizeHub(ctx, actor, c.HubID, true, ConnectorRanAction, connectorTarget, c.ID); err != nil {
				return nil, err
			}
			return s.pull(ctx, actor, c, in.Bool("full"), in.Bool("dry_run"))
		}),
	})
}

// pulled is what a run did.
type pulled struct {
	pages     int
	runs      []shared.ID
	counts    domain.Counts
	decisions []domain.Decision
	failure   string
}

// pull runs a connector's source through the reconciliation, page by page, each page in a
// transaction of its own so that no call to the source holds one open.
func (s Connectors) pull(ctx context.Context, actor appshared.ActorContext, c domain.Connector, full, dry bool) (usecase.Output, error) {
	credential, err := s.credential(ctx, actor, c)
	if err != nil {
		return nil, err
	}
	cursor := c.Cursor
	if full {
		cursor = ""
	}
	var did pulled
	var retryAt *time.Time
	pages := MaxPagesPerRun
	if dry {
		pages = 1
	}
	for did.pages < pages {
		page, err := s.Pullers.Pull(ctx, connectorport.Pull{
			Connector: c.ID, Kind: c.Kind, BaseURL: c.BaseURL, Settings: c.SettingsCopy(), Hosts: c.Hosts(),
			Credential: credential, Cursor: cursor,
		})
		if err != nil {
			did.failure = failureCode(err)
			if wait := connectorport.RetryAfter(err); wait > 0 {
				at := s.Inventory.Clock.Now().Add(wait)
				retryAt = &at
			}
			break
		}
		did.pages++
		run, err := s.reconcilePage(ctx, actor, c, page.Payload, dry)
		if err != nil {
			return nil, err
		}
		if !dry {
			did.runs = append(did.runs, run.ID)
		} else {
			did.decisions = run.Decisions
		}
		add(&did.counts, run.Counts)
		cursor = page.Cursor
		if !page.More {
			break
		}
	}
	if dry {
		return pullOutput(c, did, true), nil
	}
	outcome := domain.ConnectorOK
	switch {
	case did.failure != "":
		outcome = domain.ConnectorFailed
	case did.counts.Items == 0:
		outcome = domain.ConnectorEmpty
	}
	var last shared.ID
	if n := len(did.runs); n > 0 {
		last = did.runs[n-1]
	}
	next := c.Ran(s.Inventory.Clock.Now(), last, outcome, did.failure, cursor, retryAt)
	// A failure after some pages keeps what they reached: those pages are written.
	if did.failure != "" && len(did.runs) > 0 {
		next.Cursor = cursor
	}
	err = s.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := s.Store.RecordRun(ctx, next); err != nil {
			return err
		}
		if err := s.schedule(ctx, actor.TenantID, next); err != nil {
			return err
		}
		return s.Inventory.record(ctx, actor, ConnectorRanAction, connectorTarget, c.ID,
			audit.Change{Field: "outcome", Classification: audit.Open, To: string(outcome)},
			audit.Change{Field: "error", Classification: audit.Open, To: did.failure},
			audit.Change{Field: "pages", Classification: audit.Open, To: did.pages},
			audit.Change{Field: "items", Classification: audit.Open, To: did.counts.Items})
	})
	if err != nil {
		return nil, err
	}
	return pullOutput(next, did, false), nil
}

// failureCode is a pull's refusal by its message code; anything else is the source unreachable.
func failureCode(err error) string {
	var e *shared.Error
	if errors.As(err, &e) && e.DetailCode != "" {
		return e.DetailCode
	}
	return connectorport.CodeUnreachable
}

func (s Connectors) credential(ctx context.Context, actor appshared.ActorContext, c domain.Connector) (secret.Secret, error) {
	if !c.HasCredential {
		return secret.Secret{}, invalid("/connector_id", "cmdb.connector_credential_missing")
	}
	var stored repository.Credential
	if err := s.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		stored, err = s.Store.Credential(ctx, c.ID)
		return err
	}); err != nil {
		return secret.Secret{}, err
	}
	return s.Encryptor.Open(ctx, crypto.Sealed{KeyID: stored.KeyID, Ciphertext: stored.Ciphertext}, CredentialPurpose(c.ID))
}

func (s Connectors) reconcilePage(ctx context.Context, actor appshared.ActorContext, c domain.Connector, payload domain.Payload, dry bool) (domain.Run, error) {
	var run domain.Run
	write := s.Inventory.UnitOfWork.Within
	if dry {
		write = s.Inventory.UnitOfWork.WithinReadOnly
	}
	err := write(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		run, err = s.Inventory.reconcile(ctx, actor, c.HubID, c.Source, payload, dry)
		if err != nil || dry {
			return err
		}
		k := run.Counts
		return s.Inventory.record(ctx, actor, ReconciledAction, runTarget, run.ID,
			audit.Change{Field: "source", Classification: audit.Open, To: c.Source},
			audit.Change{Field: "connector", Classification: audit.Open, To: c.Key},
			audit.Change{Field: "created", Classification: audit.Open, To: k.Created},
			audit.Change{Field: "updated", Classification: audit.Open, To: k.Updated},
			audit.Change{Field: "held", Classification: audit.Open, To: k.Held},
			audit.Change{Field: "duplicates", Classification: audit.Open, To: k.Duplicates})
	})
	return run, err
}

func add(sum *domain.Counts, c domain.Counts) {
	sum.Items += c.Items
	sum.Created += c.Created
	sum.Updated += c.Updated
	sum.Unchanged += c.Unchanged
	sum.Held += c.Held
	sum.Duplicates += c.Duplicates
	sum.Refused += c.Refused
	sum.RelationsAdded += c.RelationsAdded
	sum.RelationsSeen += c.RelationsSeen
	sum.RelationsRefused += c.RelationsRefused
}

func pullOutput(c domain.Connector, did pulled, dry bool) usecase.Output {
	runs := make([]string, 0, len(did.runs))
	for _, id := range did.runs {
		runs = append(runs, id.String())
	}
	decisions := make([]usecase.Output, 0, len(did.decisions))
	for _, d := range did.decisions {
		decisions = append(decisions, DecisionOutput(d))
	}
	outcome := "OK"
	if did.failure != "" {
		outcome = "FAILED"
	}
	return usecase.Output{
		"connector": ConnectorOutput(c), "dry_run": dry, "pages": did.pages, "run_ids": runs,
		"counts": CountsOutput(did.counts), "decisions": decisions, "outcome": outcome, "error": did.failure,
	}
}

// RunScheduled is a connector's scheduled pull, as the person who last defined it: it answers
// when the connector is owed next, zero when it is not scheduled any more.
func (s Connectors) RunScheduled(ctx context.Context, tenantID, id shared.ID) (time.Time, error) {
	scope := appshared.ActorContext{TenantID: tenantID}.PersistenceScope()
	var c domain.Connector
	if err := s.Inventory.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		c, err = s.Store.FindConnector(ctx, id, false)
		return err
	}); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	if !c.Enabled || c.Every == 0 || c.NextRunAt == nil || c.Spec().Push {
		return time.Time{}, nil
	}
	now := s.Inventory.Clock.Now()
	if !c.Due(now) {
		return *c.NextRunAt, nil
	}
	actor := s.runAs(tenantID, c)
	if err := s.Inventory.authorizeHub(ctx, actor, c.HubID, true, ConnectorRanAction, connectorTarget, c.ID); err != nil {
		// The person it runs as lost the right to write the hub: the connector fails, as a source
		// that refuses does, and stops after its failures.
		next := c.Ran(now, "", domain.ConnectorFailed, "cmdb.connector_run_as_refused", c.Cursor, nil)
		return nextOf(next), s.Inventory.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
			return s.Store.RecordRun(ctx, next)
		})
	}
	out, err := s.pull(ctx, actor, c, false, false)
	if err != nil {
		return time.Time{}, err
	}
	conn, _ := out["connector"].(usecase.Output)
	if at, ok := conn["next_run_at"].(time.Time); ok {
		return at, nil
	}
	return time.Time{}, nil
}

func nextOf(c domain.Connector) time.Time {
	if c.NextRunAt == nil {
		return time.Time{}
	}
	return *c.NextRunAt
}

// runAs is the actor a connector acts as when nobody asked: an automation acting with the rights
// of the person who last defined it, and the item scopes only.
func (s Connectors) runAs(tenantID shared.ID, c domain.Connector) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorAutomation, TenantID: tenantID, AccountID: c.RunAs,
		Scopes: []string{cisRead, cisWrite}, Channel: string(work.ChannelImport),
	}
}

// ListCiAlerts answers a hub's alerts.
type ListCiAlerts struct{ Connectors Connectors }

// Descriptor is the catalogue entry.
func (h ListCiAlerts) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListCiAlertsName, Permission: service.PermissionRead,
		Summary: "Lists the alerts monitoring programs posted to a hub's intakes, the latest first: each problem " +
			"once by its key, firing or resolved, with its severity and title, how often it fired, the CI its " +
			"host names and the entry opened for it.",
		SideEffects: "None. Reads only.", TokenScope: cisRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "status", Kind: usecase.KindString, Enum: []string{"FIRING", "RESOLVED"}, Description: "Only alerts in this state."},
			{Name: "limit", Kind: usecase.KindInt, Description: "At most this many, up to 200; 50 when not given."},
		},
		Audit: usecase.AuditDeclaration{Action: ConnectorReadAction, TargetType: alertTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			s := h.Connectors
			if err := s.Inventory.authorizeHub(ctx, actor, hub, false, ConnectorReadAction, alertTarget, hub); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = s.Inventory.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				list, err := s.Store.ListAlerts(ctx, hub, domain.AlertStatus(in.String("status")), limitOf(in, 50, 200))
				for _, a := range list {
					rows = append(rows, AlertOutput(a))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	})
}
