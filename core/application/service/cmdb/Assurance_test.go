// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/cmdb"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// assured is the assurance store in memory.
type assured struct {
	repository.Assurance
	affected  []domain.Affected
	campaigns map[shared.ID]domain.Campaign
	asked     map[shared.ID][]domain.Attestation
	groups    []domain.Group
	assets    map[shared.ID]domain.Asset
	aliases   []domain.SoftwareAlias
}

func newAssured() *assured {
	return &assured{campaigns: map[shared.ID]domain.Campaign{}, asked: map[shared.ID][]domain.Attestation{}, assets: map[shared.ID]domain.Asset{}}
}

func (a *assured) AddAffected(_ context.Context, row domain.Affected) (bool, error) {
	a.affected = append(a.affected, row)
	return true, nil
}

func (a *assured) RemoveAffected(_ context.Context, item, ci shared.ID) (domain.Affected, error) {
	for i, r := range a.affected {
		if r.ItemID == item && r.CIID == ci {
			a.affected = append(a.affected[:i], a.affected[i+1:]...)
			return r, nil
		}
	}
	return domain.Affected{}, shared.ErrNotFound.WithDetail("cmdb.affected_not_found")
}

func (a *assured) AffectedOf(_ context.Context, item shared.ID) ([]domain.Affected, error) {
	var out []domain.Affected
	for _, r := range a.affected {
		if r.ItemID == item {
			out = append(out, r)
		}
	}
	return out, nil
}

func (a *assured) AffectingAny(_ context.Context, cis []shared.ID, _ int) ([]domain.Affected, error) {
	var out []domain.Affected
	for _, r := range a.affected {
		for _, c := range cis {
			if r.CIID == c {
				out = append(out, r)
				break
			}
		}
	}
	return out, nil
}

func (a *assured) InsertCampaign(_ context.Context, c domain.Campaign, asked []domain.Attestation) error {
	a.campaigns[c.ID], a.asked[c.ID] = c, asked
	return nil
}

func (a *assured) FindCampaign(_ context.Context, id shared.ID) (domain.Campaign, error) {
	c, ok := a.campaigns[id]
	if !ok {
		return c, shared.ErrNotFound.WithDetail("cmdb.campaign_not_found")
	}
	return c, nil
}

func (a *assured) CloseCampaign(_ context.Context, id shared.ID, at time.Time, by shared.ID, _ int) error {
	c := a.campaigns[id]
	c.ClosedAt, c.ClosedBy, c.Version = &at, by, c.Version+1
	a.campaigns[id] = c
	return nil
}

func (a *assured) Attestations(_ context.Context, id shared.ID) ([]domain.Attestation, error) {
	return a.asked[id], nil
}

func (a *assured) Tallies(_ context.Context, ids []shared.ID) (map[shared.ID]map[domain.Verdict]int, error) {
	out := map[shared.ID]map[domain.Verdict]int{}
	for _, id := range ids {
		out[id] = map[domain.Verdict]int{}
		for _, at := range a.asked[id] {
			out[id][at.Verdict]++
		}
	}
	return out, nil
}

func (a *assured) FindAttestation(_ context.Context, id, ci shared.ID) (domain.Attestation, error) {
	for _, at := range a.asked[id] {
		if at.CIID == ci {
			return at, nil
		}
	}
	return domain.Attestation{}, shared.ErrNotFound.WithDetail("cmdb.attestation_not_found")
}

func (a *assured) Answer(_ context.Context, at domain.Attestation) error {
	for i, x := range a.asked[at.CampaignID] {
		if x.CIID == at.CIID {
			a.asked[at.CampaignID][i] = at
		}
	}
	return nil
}

func (a *assured) PutGroup(_ context.Context, g domain.Group, _ int) error {
	for i, x := range a.groups {
		if x.Key == g.Key {
			a.groups[i] = g
			return nil
		}
	}
	a.groups = append(a.groups, g)
	return nil
}

func (a *assured) Groups(context.Context, shared.ID) ([]domain.Group, error) { return a.groups, nil }

func (a *assured) PutAsset(_ context.Context, x domain.Asset, expected int) error {
	if a.assets[x.CIID].Version != expected {
		return shared.ErrVersionConflict
	}
	x.Version = expected + 1
	a.assets[x.CIID] = x
	return nil
}

func (a *assured) FindAsset(_ context.Context, id shared.ID) (domain.Asset, error) {
	x, ok := a.assets[id]
	if !ok {
		return x, shared.ErrNotFound.WithDetail("cmdb.asset_not_found")
	}
	return x, nil
}

func (a *assured) PutAlias(_ context.Context, x domain.SoftwareAlias, _ int) error {
	a.aliases = append(a.aliases, x)
	return nil
}

func (a *assured) Aliases(context.Context) ([]domain.SoftwareAlias, error) { return a.aliases, nil }

func (a *assured) Installations(context.Context, shared.ID) (map[shared.ID]int, error) {
	return map[shared.ID]int{}, nil
}

func ciID(n string) shared.ID { return shared.MustParseID("01936f2a-7c1e-7000-8000-0000000f" + n) }

var (
	host     = ciID("0001")
	vm1      = ciID("0002")
	booking  = ciID("0003")
	offering = ciID("0004")
	vault    = ciID("0005")
	incident = ciID("0101")
	change1  = ciID("0102")
	change2  = ciID("0103")
	later    = ciID("0104")
)

// assuredFixture is a hub whose host carries a machine running the booking application, which
// the booking offering depends on; and a vault in the private hub.
func assuredFixture() (fixture, *assured) {
	f := newFixture()
	a := newAssured()
	f.inv.Assurance = a
	for _, ci := range []domain.CI{
		{ID: host, HubID: hub, Class: "server", Name: "host1", Stage: domain.StageInUse, Status: domain.StatusOK, Compliance: domain.Compliance{OwnerAccount: bert, Quantity: 1}},
		{ID: vm1, HubID: hub, Class: "virtual_machine", Name: "vm1", Stage: domain.StageInUse, Status: domain.StatusOK, Attributes: map[string]any{"app": "booking"}, Compliance: domain.Compliance{Quantity: 1}},
		{ID: booking, HubID: hub, Class: "application_service", Name: "Booking", Stage: domain.StageInUse, Status: domain.StatusOK, Compliance: domain.Compliance{Quantity: 1}},
		{ID: offering, HubID: hub, Class: "business_offering", Name: "Booking for reception", Stage: domain.StageInUse, Status: domain.StatusOK, Compliance: domain.Compliance{Quantity: 1}},
		{ID: vault, HubID: private, Class: "server", Name: "vault", Stage: domain.StageInUse, Status: domain.StatusOK, Compliance: domain.Compliance{Quantity: 1}},
	} {
		ci.Version = 1
		f.m.cis[ci.ID] = ci
	}
	for i, r := range []domain.Relation{
		{Type: domain.RelHostedOn, SourceCI: vm1, TargetCI: host},
		{Type: domain.RelRunsOn, SourceCI: booking, TargetCI: vm1},
		{Type: domain.RelDependsOn, SourceCI: offering, TargetCI: booking},
	} {
		r.ID, r.HubID = ciID("100"+string(rune('0'+i))), hub
		f.m.relations[r.ID] = r
	}
	return f, a
}

type plannedEntries map[shared.ID]workrepo.EntrySummary

func (p plannedEntries) Entries(_ context.Context, ids []shared.ID) ([]workrepo.EntrySummary, error) {
	var out []workrepo.EntrySummary
	for _, id := range ids {
		if e, ok := p[id]; ok {
			out = append(out, e)
		}
	}
	return out, nil
}

func at(day int) *time.Time {
	t := start.AddDate(0, 0, day)
	return &t
}

// UC-AST-09 checks 1-4: an entry names the CIs it is about, sees the services their failure
// reaches and the other open entries on them, and a change planned on a CI it reaches in an
// overlapping window collides; a CI out of the writer's reach cannot be named.
func TestAnEntrysAffectedCIsShowTheirImpactAndCollisions(t *testing.T) {
	f, a := assuredFixture()
	entries := plannedEntries{
		incident: {ID: incident, HubID: hub, CollectionID: collected, Title: "Booking is slow"},
		change1:  {ID: change1, HubID: hub, CollectionID: collected, Title: "Patch host1", StartAt: at(1), DueAt: at(2)},
		change2:  {ID: change2, HubID: hub, CollectionID: collected, Title: "Resize vm1", StartAt: at(2), DueAt: at(3)},
		later:    {ID: later, HubID: hub, CollectionID: collected, Title: "Move booking", StartAt: at(9), DueAt: at(10)},
	}
	aff := Affecting{Inventory: f.inv, Pointers: Pointers{Entries: entries, Reader: seeing{}}}
	affect := AffectEntryCi{Affecting: aff}.Descriptor()
	for _, pair := range [][2]shared.ID{{incident, booking}, {change1, host}, {change2, vm1}, {later, booking}} {
		if _, err := call(t, affect, as(anna), usecase.Input{"item_id": pair[0].String(), "ci_id": pair[1].String()}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := call(t, affect, as(anna), usecase.Input{"item_id": incident.String(), "ci_id": booking.String()}); err != nil || len(a.affected) != 4 {
		t.Fatalf("naming a CI again: %v, %d rows", err, len(a.affected))
	}
	out, err := call(t, ListEntryCis{Affecting: aff}.Descriptor(), as(bert), usecase.Input{"item_id": change1.String()})
	if err != nil {
		t.Fatal(err)
	}
	if cis := out["cis"].([]usecase.Output); len(cis) != 1 || cis[0]["ci_id"] != host.String() {
		t.Errorf("the change's CIs: %v", cis)
	}
	var services []string
	for _, s := range out["services"].([]usecase.Output) {
		services = append(services, s["name"].(string))
	}
	if len(services) != 2 {
		t.Errorf("the services host1's failure reaches: %v", services)
	}
	related := map[string]bool{}
	for _, r := range out["related"].([]usecase.Output) {
		related[r["item_id"].(string)] = true
	}
	if !related[incident.String()] || !related[change2.String()] || !related[later.String()] || len(related) != 3 {
		t.Errorf("the entries on the CIs host1 reaches: %v", related)
	}
	collisions := out["collisions"].([]usecase.Output)
	if len(collisions) != 1 || collisions[0]["item_id"] != change2.String() || collisions[0]["direct"] != false {
		t.Errorf("collisions: %v", collisions)
	}

	if _, err := call(t, affect, as(bert), usecase.Input{"item_id": incident.String(), "ci_id": vault.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("Bert named a CI of a hub out of his reach: %v", err)
	}
	if _, err := call(t, UnaffectEntryCi{Affecting: aff}.Descriptor(), as(anna), usecase.Input{"item_id": later.String(), "ci_id": booking.String()}); err != nil || len(a.affected) != 3 {
		t.Errorf("removing: %v", err)
	}
}

// UC-AST-10 checks 1-4: a campaign asks the active CIs of its classes, the person asked answers,
// a confirmed CI's review moves a year on, a disputed one falls due today, and the score follows.
func TestACampaignAsksOwnersAndMovesTheReviewByTheAnswer(t *testing.T) {
	f, a := assuredFixture()
	out, err := call(t, StartCiCampaign{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{
		"hub_id": hub.String(), "name": "Annual review", "classes": []any{"server", "virtual_machine"}, "due_on": "2026-11-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := shared.ID(out["id"].(string))
	if score := out["score"].(usecase.Output); score["total"] != 2 {
		t.Fatalf("asked: %v", score)
	}
	attest := AttestCi{Inventory: f.inv}.Descriptor()
	if _, err := call(t, attest, as(bert), usecase.Input{"campaign_id": id.String(), "ci_id": host.String(), "verdict": "CONFIRMED"}); err != nil {
		t.Fatal(err)
	}
	if due := f.m.cis[host].Compliance.ReviewDue; due == nil || due.Format(time.DateOnly) != "2027-10-29" {
		t.Errorf("host1's next review: %v", due)
	}
	if _, err := call(t, attest, as(anna), usecase.Input{"campaign_id": id.String(), "ci_id": vm1.String(), "verdict": "DISPUTED"}); err != nil {
		t.Fatal(err)
	}
	if due := f.m.cis[vm1].Compliance.ReviewDue; due == nil || due.Format(time.DateOnly) != "2026-10-29" {
		t.Errorf("vm1's review: %v", due)
	}
	read, err := call(t, ReadCiCampaign{Inventory: f.inv}.Descriptor(), as(bert), usecase.Input{"campaign_id": id.String()})
	if err != nil {
		t.Fatal(err)
	}
	if score := read["score"].(usecase.Output); score["confirmed"] != 1 || score["disputed"] != 1 || score["rate"] != 1.0 {
		t.Errorf("score: %v", score)
	}
	if _, err := call(t, CloseCiCampaign{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{"campaign_id": id.String()}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, attest, as(bert), usecase.Input{"campaign_id": id.String(), "ci_id": host.String(), "verdict": "DISPUTED"}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a closed campaign took an answer: %v", err)
	}
	if len(a.campaigns) != 1 {
		t.Errorf("campaigns: %d", len(a.campaigns))
	}
}

// UC-AST-11 checks 1-3: a group holds what its filter matches, a group standing for a service
// brings its members into the service map, and the map places each CI in its layer.
func TestAGroupStandsForAServiceInTheServiceMap(t *testing.T) {
	f, _ := assuredFixture()
	out, err := call(t, DefineCiGroup{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{
		"hub_id": hub.String(), "key": "booking_vms", "name": "Booking machines",
		"filter":     map[string]any{"classes": []any{"virtual_machine"}, "attributes": map[string]any{"app": "Booking"}},
		"service_id": booking.String(),
	})
	if err != nil || out["members"] != 1 {
		t.Fatalf("group: %v %v", out, err)
	}
	m, err := call(t, ReadServiceMap{Inventory: f.inv}.Descriptor(), as(bert), usecase.Input{"hub_id": hub.String()})
	if err != nil {
		t.Fatal(err)
	}
	layers := map[string]int{}
	for _, n := range m["nodes"].([]usecase.Output) {
		layers[n["name"].(string)] = n["layer"].(int)
	}
	if layers["Booking for reception"] != 0 || layers["Booking"] != 2 || layers["vm1"] != 4 || layers["host1"] != 5 {
		t.Errorf("layers: %v", layers)
	}
	var grouped bool
	for _, e := range m["edges"].([]usecase.Output) {
		grouped = grouped || e["type"] == "group:booking_vms" && e["target"] == vm1.String()
	}
	if !grouped {
		t.Errorf("the group's membership is not drawn: %v", m["edges"])
	}
}

// UC-AST-12 checks 1-3: an asset is registered for a CI, its CI's stage follows its state, and it
// is disposed of only with a certificate.
func TestAnAssetLeadsItsCIsStageToItsDisposal(t *testing.T) {
	f, a := assuredFixture()
	out, err := call(t, RegisterCiAsset{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{
		"ci_id": host.String(), "asset_tag": "INV-0042", "cost_minor": 249900, "currency": "EUR",
		"purchased_on": "2024-03-01", "warranty_until": "2026-03-01", "state": "IN_STOCK",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["warranty_ended"] != true || f.m.cis[host].Stage != domain.StageInStock {
		t.Errorf("registered: %v, stage %s", out, f.m.cis[host].Stage)
	}
	move := MoveCiAsset{Inventory: f.inv}.Descriptor()
	if _, err := call(t, move, as(anna), usecase.Input{"ci_id": host.String(), "state": "RETIRED"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, move, as(anna), usecase.Input{"ci_id": host.String(), "state": "DISPOSED", "disposal": map[string]any{"method": "DESTROYED"}}); err == nil {
		t.Error("disposed without a certificate")
	}
	if _, err := call(t, move, as(anna), usecase.Input{"ci_id": host.String(), "state": "DISPOSED", "disposal": map[string]any{"method": "DESTROYED", "certificate": "C-17"}}); err != nil {
		t.Fatal(err)
	}
	if f.m.cis[host].Stage != domain.StageDisposed || a.assets[host].Disposal.Certificate != "C-17" {
		t.Errorf("disposed: %s %+v", f.m.cis[host].Stage, a.assets[host].Disposal)
	}
	if _, err := call(t, RegisterCiAsset{Inventory: f.inv}.Descriptor(), as(bert), usecase.Input{"ci_id": vault.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("Bert registered an asset in a hub out of reach: %v", err)
	}
}

// UC-AST-13 checks 1-2: software is counted by its folded product, an alias names it.
func TestSoftwareIsCountedByItsProductAndAnAliasNamesIt(t *testing.T) {
	f, _ := assuredFixture()
	for i, name := range []string{"Code", "Mozilla Firefox 128.0 (x64)", "mozilla firefox"} {
		id := ciID("020" + string(rune('0'+i)))
		f.m.cis[id] = domain.CI{ID: id, HubID: hub, Class: "software", Name: name, Stage: domain.StageInUse, Status: domain.StatusOK}
	}
	if _, err := call(t, DefineSoftwareAlias{Inventory: f.inv}.Descriptor(), as(anna), usecase.Input{
		"match_product": "code", "publisher": "Microsoft", "product": "Visual Studio Code",
	}); err != nil {
		t.Fatal(err)
	}
	out, err := call(t, ListSoftwareProducts{Inventory: f.inv}.Descriptor(), as(bert), usecase.Input{"hub_id": hub.String()})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, p := range out["data"].([]usecase.Output) {
		got[p["product"].(string)] = len(p["ci_ids"].([]string))
	}
	if got["Visual Studio Code"] != 1 || got["mozilla firefox"] != 2 {
		t.Errorf("products: %v", got)
	}
}
