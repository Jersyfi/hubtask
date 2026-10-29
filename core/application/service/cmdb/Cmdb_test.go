// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/cmdb"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenant  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d001")
	anna    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d002")
	bert    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d003")
	hub     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d004")
	private = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d005")
	start   = time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
)

type passthrough struct{}

func (passthrough) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (passthrough) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type clockAt struct{ now *time.Time }

func (c clockAt) Now() time.Time { return *c.now }

type counter struct{ n int }

func (c *counter) NewID() shared.ID {
	c.n++
	return shared.MustParseID(fmt.Sprintf("01936f2a-7c1e-7000-8000-%012x", c.n))
}

type trail struct{ entries []audit.Entry }

func (t *trail) Append(_ context.Context, e audit.Entry) error {
	t.entries = append(t.entries, e)
	return nil
}

// gate lets Anna do everything; Bert reaches no private hub.
type gate struct{}

func (gate) Authorize(_ context.Context, actor appshared.ActorContext, r access.Request) error {
	for _, s := range r.Path {
		if s.ID == private && actor.AccountID != anna {
			return shared.ErrNotFound.WithDetail("containers.not_found")
		}
	}
	return nil
}

type hubs struct{ workrepo.Containers }

func (hubs) Find(_ context.Context, id shared.ID) (work.Container, error) {
	if id != hub && id != private {
		return work.Container{}, shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return work.Container{ID: id, Type: work.ContainerHub}, nil
}

type memory struct {
	repository.Store
	classes   []domain.Class
	sources   []domain.Source
	cis       map[shared.ID]domain.CI
	ids       map[shared.ID]map[string]string
	relations map[shared.ID]domain.Relation
	runs      []domain.Run
	holds     []domain.Hold
	dups      []domain.Duplicate
}

func newMemory() *memory {
	return &memory{cis: map[shared.ID]domain.CI{}, ids: map[shared.ID]map[string]string{}, relations: map[shared.ID]domain.Relation{}}
}

func (m *memory) Classes(context.Context) ([]domain.Class, error) { return m.classes, nil }
func (m *memory) RelationTypes(context.Context) ([]domain.RelationType, error) {
	return nil, nil
}
func (m *memory) PutClass(_ context.Context, c domain.Class, _ int) error {
	m.classes = append(m.classes, c)
	return nil
}
func (m *memory) Sources(context.Context) ([]domain.Source, error) { return m.sources, nil }
func (m *memory) PutSource(_ context.Context, s domain.Source, _ int) error {
	m.sources = append(m.sources, s)
	return nil
}
func (m *memory) TouchSource(context.Context, string, time.Time) error { return nil }
func (m *memory) InsertCI(_ context.Context, ci domain.CI) error {
	m.cis[ci.ID] = ci
	return nil
}
func (m *memory) UpdateCI(_ context.Context, ci domain.CI, expected int) error {
	if m.cis[ci.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	ci.Version = expected + 1
	m.cis[ci.ID] = ci
	return nil
}
func (m *memory) FindCI(_ context.Context, id shared.ID, _ bool) (domain.CI, error) {
	ci, ok := m.cis[id]
	if !ok {
		return domain.CI{}, shared.ErrNotFound.WithDetail("cmdb.ci_not_found")
	}
	return ci, nil
}
func (m *memory) ListCIs(_ context.Context, h shared.ID, classes []string, after shared.ID, limit int) ([]domain.CI, error) {
	var out []domain.CI
	for _, ci := range m.cis {
		if ci.HubID == h && ci.ID > after && (len(classes) == 0 || slices.Contains(classes, ci.Class)) {
			out = append(out, ci)
		}
	}
	slices.SortFunc(out, func(a, b domain.CI) int { return strings.Compare(string(a.ID), string(b.ID)) })
	return out[:min(limit, len(out))], nil
}
func (m *memory) CIsByID(_ context.Context, list []shared.ID) ([]domain.CI, error) {
	var out []domain.CI
	for _, id := range list {
		if ci, ok := m.cis[id]; ok {
			out = append(out, ci)
		}
	}
	return out, nil
}
func (m *memory) CountCIs(_ context.Context, h shared.ID) (int, error) { return len(m.cis), nil }
func (m *memory) PutIdentifiers(_ context.Context, ci domain.CI, ids map[string]string) error {
	next := map[string]string{}
	for k, v := range m.ids[ci.ID] {
		if strings.HasPrefix(k, "source:") {
			next[k] = v
		}
	}
	for k, v := range ids {
		next[k] = v
	}
	m.ids[ci.ID] = next
	return nil
}
func (m *memory) FindIdentifiers(_ context.Context, h shared.ID, want map[string]string) ([]domain.IdentifierRow, error) {
	var out []domain.IdentifierRow
	for id, ids := range m.ids {
		ci := m.cis[id]
		if ci.HubID != h || !ci.MergedInto.IsZero() {
			continue
		}
		for k, v := range ids {
			if want[k] == v {
				out = append(out, domain.IdentifierRow{CI: id, Class: ci.Class, Name: k, Value: v})
			}
		}
	}
	return out, nil
}
func (m *memory) MergeInto(_ context.Context, survivor, dup shared.ID) error {
	for k, v := range m.ids[dup] {
		if strings.HasPrefix(k, "source:") {
			m.ids[survivor][k] = v
		}
	}
	delete(m.ids, dup)
	for id, r := range m.relations {
		if r.SourceCI == dup {
			r.SourceCI = survivor
		}
		if r.TargetCI == dup {
			r.TargetCI = survivor
		}
		m.relations[id] = r
	}
	return nil
}
func (m *memory) PutRelation(_ context.Context, r domain.Relation) (shared.ID, bool, error) {
	for id, old := range m.relations {
		if old.Type == r.Type && old.SourceCI == r.SourceCI && old.TargetCI == r.TargetCI {
			return id, false, nil
		}
	}
	m.relations[r.ID] = r
	return r.ID, true, nil
}
func (m *memory) FindRelation(_ context.Context, id shared.ID) (domain.Relation, error) {
	return m.relations[id], nil
}
func (m *memory) DeleteRelation(_ context.Context, id shared.ID) error {
	delete(m.relations, id)
	return nil
}
func (m *memory) RelationsOf(_ context.Context, ci shared.ID) ([]domain.Relation, error) {
	var out []domain.Relation
	for _, r := range m.relations {
		if r.SourceCI == ci || r.TargetCI == ci {
			out = append(out, r)
		}
	}
	return out, nil
}
func (m *memory) HubRelations(context.Context, shared.ID) ([]domain.Relation, error) {
	var out []domain.Relation
	for _, r := range m.relations {
		out = append(out, r)
	}
	return out, nil
}
func (m *memory) Walk(_ context.Context, from shared.ID, down bool, src, tgt []string, depth int) ([]repository.Reached, error) {
	seen := map[shared.ID]bool{from: true}
	frontier, out := []shared.ID{from}, []repository.Reached{}
	for d := 1; d <= depth && len(frontier) > 0; d++ {
		var next []shared.ID
		for _, at := range frontier {
			for _, r := range m.relations {
				var to shared.ID
				switch {
				case down && r.TargetCI == at && slices.Contains(src, r.Type), !down && r.TargetCI == at && slices.Contains(tgt, r.Type):
					to = r.SourceCI
				case down && r.SourceCI == at && slices.Contains(tgt, r.Type), !down && r.SourceCI == at && slices.Contains(src, r.Type):
					to = r.TargetCI
				}
				if to != "" && !seen[to] {
					seen[to] = true
					next = append(next, to)
					out = append(out, repository.Reached{CI: to, Depth: d})
				}
			}
		}
		frontier = next
	}
	return out, nil
}
func (m *memory) InsertRun(_ context.Context, r domain.Run) error {
	m.runs = append(m.runs, r)
	return nil
}
func (m *memory) InsertHold(_ context.Context, h domain.Hold) error {
	m.holds = append(m.holds, h)
	return nil
}
func (m *memory) OpenDuplicate(_ context.Context, d domain.Duplicate) (bool, error) {
	m.dups = append(m.dups, d)
	return true, nil
}
func (m *memory) Duplicates(context.Context, shared.ID, int) ([]domain.Duplicate, error) {
	return m.dups, nil
}
func (m *memory) FindDuplicate(_ context.Context, id shared.ID) (domain.Duplicate, error) {
	for _, d := range m.dups {
		if d.ID == id {
			return d, nil
		}
	}
	return domain.Duplicate{}, shared.ErrNotFound
}
func (m *memory) ResolveDuplicate(_ context.Context, id shared.ID, _ time.Time, _, _ shared.ID) error {
	m.dups = slices.DeleteFunc(m.dups, func(d domain.Duplicate) bool { return d.ID == id })
	return nil
}

type fixture struct {
	m   *memory
	inv Inventory
	now *time.Time
}

func newFixture() fixture {
	now := start
	m := newMemory()
	m.sources = []domain.Source{
		{Key: "netbox", Kind: domain.SourceConnector, Rank: 50, MayCreate: true},
		{Key: "scan", Kind: domain.SourceAPI, Rank: 100, MayCreate: true},
		{Key: "audit", Kind: domain.SourceFile, Rank: 200},
		{Key: "sheet", Kind: domain.SourceFile, Rank: 150, MayCreate: true},
	}
	return fixture{m: m, now: &now, inv: Inventory{
		Store: m, Containers: hubs{}, Authorizer: gate{}, Audit: &trail{}, UnitOfWork: passthrough{},
		Clock: clockAt{&now}, IDs: &counter{},
	}}
}

func as(account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{TenantID: tenant, AccountID: account, Kind: shared.ActorUser}
}

func call(t *testing.T, d usecase.Descriptor, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	t.Helper()
	return d.Handler.Invoke(context.Background(), actor, in)
}

func items(list ...map[string]any) []any {
	out := make([]any, len(list))
	for i, l := range list {
		out[i] = l
	}
	return out
}

func outcomes(out usecase.Output) []string {
	var got []string
	for _, d := range out["decisions"].([]usecase.Output) {
		got = append(got, d["outcome"].(string))
	}
	return got
}

// A source's rerun finds the CIs it made by its own key, and a payload without an identifier is
// held, never inserted (UC-AST-02 checks 1 and 2).
func TestAReconciliationIsIdempotentAndHoldsWhatItCannotIdentify(t *testing.T) {
	f := newFixture()
	payload := usecase.Input{"hub_id": hub.String(), "source": "netbox", "items": items(
		map[string]any{"ref": "a", "class": "server", "name": "web1", "native_key": "7", "attributes": map[string]any{"model": "R650"}},
		map[string]any{"ref": "b", "class": "server", "name": "nameless"},
	)}
	out, err := call(t, ReconcileCis{f.inv}.Descriptor(), as(anna), payload)
	if err != nil || !slices.Equal(outcomes(out), []string{"CREATE", "HOLD"}) {
		t.Fatalf("first run %v, %v", outcomes(out), err)
	}
	if len(f.m.holds) != 1 || f.m.holds[0].Reason != domain.HoldInsufficient {
		t.Fatalf("the nameless server is held, got %+v", f.m.holds)
	}
	out, _ = call(t, ReconcileCis{f.inv}.Descriptor(), as(anna), payload)
	if !slices.Equal(outcomes(out), []string{"UNCHANGED", "HOLD"}) || len(f.m.cis) != 1 {
		t.Fatalf("the rerun changes nothing, got %v and %d CIs", outcomes(out), len(f.m.cis))
	}
	if d := out["decisions"].([]usecase.Output)[0]; d["rule"] != "native_key" {
		t.Fatalf("matched by the source's own key, got %v", d["rule"])
	}
}

// A source of a higher rank number does not overwrite what a lower one wrote, and the decision
// names who kept it (UC-AST-02 check 3).
func TestPrecedenceKeepsWhatTheBetterSourceWrote(t *testing.T) {
	f := newFixture()
	_, _ = call(t, ReconcileCis{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "source": "netbox", "items": items(
		map[string]any{"class": "server", "name": "web1", "attributes": map[string]any{"mac_address": "aa:bb", "model": "R650"}})})
	out, err := call(t, ReconcileCis{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "source": "scan", "items": items(
		map[string]any{"class": "server", "name": "web1", "attributes": map[string]any{"mac_address": "AA-BB", "model": "R640", "operating_system": "Debian"}})})
	if err != nil {
		t.Fatal(err)
	}
	d := out["decisions"].([]usecase.Output)[0]
	kept := d["kept"].([]usecase.Output)
	keptModel := slices.ContainsFunc(kept, func(k usecase.Output) bool { return k["attribute"] == "model" && k["by"] == "netbox" })
	if d["outcome"] != "UPDATE" || d["rule"] != "mac_address" || !keptModel || slices.ContainsFunc(kept, func(k usecase.Output) bool { return k["attribute"] == "name" }) {
		t.Fatalf("decision %+v", d)
	}
	for _, ci := range f.m.cis {
		if ci.Attributes["model"] != "R650" || ci.Attributes["operating_system"] != "Debian" {
			t.Fatalf("CI %+v", ci.Attributes)
		}
	}
}

// A source that may not create holds what it cannot find, and a dry run writes nothing
// (UC-AST-02 checks 4 and 5).
func TestADryRunWritesNothingAndAnUpdateOnlySourceHolds(t *testing.T) {
	f := newFixture()
	in := usecase.Input{"hub_id": hub.String(), "source": "netbox", "dry_run": true, "items": items(
		map[string]any{"class": "server", "name": "web1", "native_key": "7"})}
	out, err := call(t, ReconcileCis{f.inv}.Descriptor(), as(anna), in)
	if err != nil || !slices.Equal(outcomes(out), []string{"CREATE"}) || len(f.m.cis) != 0 || len(f.m.runs) != 0 || out["id"] != nil {
		t.Fatalf("dry run %v, %v; %d CIs", outcomes(out), err, len(f.m.cis))
	}
	out, _ = call(t, ReconcileCis{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "source": "audit", "items": items(
		map[string]any{"class": "server", "name": "web9", "native_key": "9"})})
	if !slices.Equal(outcomes(out), []string{"HOLD"}) || f.m.holds[0].Reason != domain.HoldMayNotCreate {
		t.Fatalf("update-only source %v", outcomes(out))
	}
}

// Two CIs one item names open a duplicate and nothing is written; merging keeps the survivor and
// retires the other as merged, never deleting it (UC-AST-03).
func TestADuplicateIsOpenedAndMergedWithoutDeleting(t *testing.T) {
	f := newFixture()
	for _, name := range []string{"one", "two"} {
		if _, err := call(t, CreateCi{f.inv}.Descriptor(), as(anna), usecase.Input{
			"hub_id": hub.String(), "class": "server", "name": name, "attributes": map[string]any{"asset_tag": "T-" + name},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for id := range f.m.cis {
		f.m.ids[id]["mac_address"] = "aa:bb"
	}
	out, _ := call(t, ReconcileCis{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "source": "netbox", "items": items(
		map[string]any{"class": "server", "name": "x", "attributes": map[string]any{"mac_address": "aa:bb"}})})
	if !slices.Equal(outcomes(out), []string{"DUPLICATE"}) || len(f.m.dups) != 1 {
		t.Fatalf("duplicate %v, %d open", outcomes(out), len(f.m.dups))
	}
	dup := f.m.dups[0]
	survivor := dup.CIs[0]
	merged, err := call(t, ResolveCiDuplicate{f.inv}.Descriptor(), as(anna), usecase.Input{"duplicate_id": dup.ID.String(), "survivor_id": survivor.String()})
	if err != nil || merged["merged"] != true {
		t.Fatalf("merge %v, %v", merged, err)
	}
	other := f.m.cis[dup.CIs[1]]
	if other.MergedInto != survivor || other.Stage != domain.StageRetired || other.Status != domain.StatusMerged || len(f.m.cis) != 2 {
		t.Fatalf("the other is retired as merged and kept: %+v", other)
	}
}

// A person cannot make a second CI the hub's identifiers already name (UC-AST-01 check 2).
func TestCreatingACIThatExistsNamesIt(t *testing.T) {
	f := newFixture()
	in := usecase.Input{"hub_id": hub.String(), "class": "server", "name": "web1", "attributes": map[string]any{"serial_number": "S1", "manufacturer": "Dell"}}
	first, err := call(t, CreateCi{f.inv}.Descriptor(), as(anna), in)
	if err != nil {
		t.Fatal(err)
	}
	in["name"] = "web1-again"
	_, err = call(t, CreateCi{f.inv}.Descriptor(), as(anna), in)
	var e *shared.Error
	if !errors.As(err, &e) || e.DetailCode != "cmdb.ci_exists" || e.Params["ci_id"] != first["id"] {
		t.Fatalf("got %v", err)
	}
	if _, err := call(t, CreateCi{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "class": "hardware", "name": "x"}); err == nil {
		t.Fatal("an abstract class holds no CI")
	}
}

// A private hub's inventory is not found by anybody else (D14).
func TestAPrivateHubsInventoryIsItsOwners(t *testing.T) {
	f := newFixture()
	if _, err := call(t, ListCis{f.inv}.Descriptor(), as(bert), usecase.Input{"hub_id": private.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := call(t, ReconcileCis{f.inv}.Descriptor(), as(bert), usecase.Input{"hub_id": private.String(), "source": "netbox", "items": []any{}}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

// A file is read by its mapping, and a bill of materials becomes software with its dependencies
// (UC-AST-04).
func TestAFileIsImportedByItsMapping(t *testing.T) {
	f := newFixture()
	csv := "Host,Serial,Vendor\nweb1,S1,Dell\n,S2,HP\nweb2,S3,HP\n"
	out, err := call(t, ImportCiFile{f.inv}.Descriptor(), as(anna), usecase.Input{
		"hub_id": hub.String(), "source": "sheet", "format": "CSV", "content": csv,
		"mapping": map[string]any{"class": "server", "name_column": "Host", "native_key_column": "Serial",
			"columns": map[string]any{"Serial": "serial_number", "Vendor": "manufacturer"}},
	})
	if err != nil || !slices.Equal(outcomes(out), []string{"CREATE", "CREATE"}) || !slices.Equal(out["skipped_rows"].([]int), []int{2}) {
		t.Fatalf("csv %v %v %v", outcomes(out), out["skipped_rows"], err)
	}
	bom := `{"bomFormat":"CycloneDX","components":[{"bom-ref":"a","name":"shop","version":"1.0","purl":"pkg:npm/shop@1.0"},
	{"bom-ref":"b","name":"left-pad","version":"1.3.0","purl":"pkg:npm/left-pad@1.3.0"}],"dependencies":[{"ref":"a","dependsOn":["b"]}]}`
	out, err = call(t, ImportCiFile{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "source": "sheet", "format": "CYCLONEDX", "content": bom})
	if err != nil || !slices.Equal(outcomes(out), []string{"CREATE", "CREATE"}) || out["counts"].(usecase.Output)["relations_added"] != 1 {
		t.Fatalf("bom %v %v %v", outcomes(out), out["counts"], err)
	}
}

// The impact of a server reaches the application running on it, and its protection need is
// inherited from it; health finds the orphan and the CI without an owner (UC-AST-05, UC-AST-06).
func TestImpactProtectionAndHealth(t *testing.T) {
	f := newFixture()
	mk := func(class, name string) shared.ID {
		out, err := call(t, CreateCi{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String(), "class": class, "name": name})
		if err != nil {
			t.Fatal(err)
		}
		return shared.MustParseID(out["id"].(string))
	}
	app, vm, lonely := mk("application_service", "Shop"), mk("virtual_machine", "web1"), mk("application_service", "Lonely")
	if _, err := call(t, RelateCis{f.inv}.Descriptor(), as(anna), usecase.Input{"source_ci_id": app.String(), "target_ci_id": vm.String(), "type": "runs_on"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, SetCiCompliance{f.inv}.Descriptor(), as(anna), usecase.Input{"ci_id": app.String(),
		"protection": map[string]any{"a": "VERY_HIGH"}, "owner_group_id": anna.String()}); err != nil {
		t.Fatal(err)
	}
	impact, err := call(t, ReadCiImpact{f.inv}.Descriptor(), as(anna), usecase.Input{"ci_id": vm.String()})
	reached := impact["reached"].([]usecase.Output)
	if err != nil || len(reached) != 1 || reached[0]["ci_id"] != app.String() || len(impact["services"].([]string)) != 1 {
		t.Fatalf("impact %+v, %v", impact, err)
	}
	read, _ := call(t, ReadCi{f.inv}.Descriptor(), as(anna), usecase.Input{"ci_id": vm.String()})
	if read["effective_protection"].(usecase.Output)["a"] != "VERY_HIGH" {
		t.Fatalf("the VM inherits the application's need, got %+v", read["effective_protection"])
	}
	health, err := call(t, ReadCmdbHealth{f.inv}.Descriptor(), as(anna), usecase.Input{"hub_id": hub.String()})
	if err != nil || health["orphans"] != 2 || health["without_owner"] != 2 || health["total"] != 3 {
		t.Fatalf("health %+v, %v (lonely %s)", health, err, lonely)
	}
	if _, err := call(t, RelateCis{f.inv}.Descriptor(), as(anna), usecase.Input{"source_ci_id": vm.String(), "target_ci_id": app.String(), "type": "offering_of"}); err == nil {
		t.Fatal("a VM is no offering of an application")
	}
}
