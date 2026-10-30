// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func day(d int) *time.Time {
	t := now.AddDate(0, 0, d)
	return &t
}

func TestAChangeCollidesOnTheSameOrAReachedCIInAnOverlappingWindow(t *testing.T) {
	self := Window{Start: day(1), End: day(3)}
	own := map[shared.ID]bool{id("01"): true}
	reached := map[shared.ID]bool{id("02"): true}
	neighbours := []Neighbour{
		{ItemID: id("10"), CIID: id("01"), Window: Window{Start: day(3), End: day(4)}}, // same CI, touching day
		{ItemID: id("11"), CIID: id("02"), Window: Window{Start: day(0), End: day(1)}}, // reached CI
		{ItemID: id("12"), CIID: id("01"), Window: Window{Start: day(5), End: day(6)}}, // later
		{ItemID: id("13"), CIID: id("09"), Window: Window{Start: day(1), End: day(2)}}, // unrelated CI
		{ItemID: id("14"), CIID: id("01"), Window: Window{Start: day(2)}},              // no window
		{ItemID: id("11"), CIID: id("01"), Window: Window{Start: day(0), End: day(1)}}, // again, now direct
		{ItemID: id("99"), CIID: id("01"), Window: Window{Start: day(1), End: day(2)}}, // itself
	}
	got := Collisions(id("99"), self, own, reached, neighbours)
	if len(got) != 2 || got[0].ItemID != id("10") || !got[0].Direct || got[1].ItemID != id("11") || !got[1].Direct {
		t.Fatalf("collisions: %+v", got)
	}
	if Collisions(id("99"), Window{Start: day(1)}, own, reached, neighbours) != nil {
		t.Fatal("an entry without a window collides with nothing")
	}
}

func TestACampaignAsksTheActiveCIsOfItsClassesDueForReview(t *testing.T) {
	classes := NewClasses(nil)
	c, err := NewCampaign(id("01"), id("02"), id("03"), id("04"), CampaignInput{
		Name: "Annual review", Classes: []string{"computer"}, DueBy: day(30), DueOn: *day(14),
	}, classes, now)
	if err != nil || c.ReviewMonths != DefaultReviewMonths {
		t.Fatalf("campaign: %+v %v", c, err)
	}
	cases := []struct {
		name string
		ci   CI
		want bool
	}{
		{"a server never reviewed", CI{Class: "server", Stage: StageInUse}, true},
		{"a laptop due in a week", CI{Class: "laptop", Stage: StageInUse, Compliance: Compliance{ReviewDue: day(7)}}, true},
		{"a server reviewed next year", CI{Class: "server", Stage: StageInUse, Compliance: Compliance{ReviewDue: day(300)}}, false},
		{"a retired server", CI{Class: "server", Stage: StageRetired}, false},
		{"a merged server", CI{Class: "server", Stage: StageInUse, Status: StatusMerged}, false},
		{"a virtual machine", CI{Class: "virtual_machine", Stage: StageInUse}, false},
	}
	for _, tc := range cases {
		if got := c.InScope(tc.ci, classes); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
	for name, in := range map[string]CampaignInput{
		"no name":       {DueOn: *day(1)},
		"due today":     {Name: "x", DueOn: now},
		"unknown class": {Name: "x", Classes: []string{"spaceship"}, DueOn: *day(1)},
		"period zero+":  {Name: "x", DueOn: *day(1), ReviewMonths: 61},
	} {
		if _, err := NewCampaign(id("01"), id("02"), id("03"), id("04"), in, classes, now); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestACampaignIsScoredByItsAnswersAndClosedTakesNone(t *testing.T) {
	c := Campaign{DueOn: *day(1), ReviewMonths: 12}
	a, err := c.Answer(Attestation{Verdict: VerdictPending}, VerdictConfirmed, id("05"), now)
	if err != nil || a.Verdict != VerdictConfirmed || a.AttestedBy != id("05") {
		t.Fatalf("answer: %+v %v", a, err)
	}
	if _, err := c.Answer(a, VerdictPending, id("05"), now); err == nil {
		t.Fatal("pending is no answer")
	}
	s := c.Tally([]Attestation{a, {Verdict: VerdictDisputed}, {Verdict: VerdictPending}, {Verdict: VerdictPending}}, now.AddDate(0, 0, 2))
	if s.Total != 4 || s.Confirmed != 1 || s.Disputed != 1 || s.Pending != 2 || s.Overdue != 2 || s.Rate != 0.5 {
		t.Fatalf("score: %+v", s)
	}
	if next := c.NextReview(now); next.Format(time.DateOnly) != "2027-10-29" {
		t.Fatalf("next review: %s", next)
	}
	closed := now
	c.ClosedAt = &closed
	if _, err := c.Answer(Attestation{}, VerdictConfirmed, id("05"), now); err == nil {
		t.Fatal("a closed campaign took an answer")
	}
}

func TestAGroupHoldsTheCIsItsFilterMatches(t *testing.T) {
	classes := NewClasses(nil)
	g, err := NewGroup(id("01"), id("02"), id("03"), id("04"), GroupInput{
		Key: "crm_vms", Name: "CRM machines",
		Filter: Filter{Classes: []string{"virtual_machine"}, Stages: []Stage{StageInUse}, Attributes: map[string]string{"app": "CRM"}},
	}, classes, now)
	if err != nil {
		t.Fatal(err)
	}
	cis := []CI{
		{ID: id("10"), Class: "virtual_machine", Stage: StageInUse, Attributes: map[string]any{"app": "crm "}},
		{ID: id("11"), Class: "virtual_machine", Stage: StageInUse, Attributes: map[string]any{"app": "erp"}},
		{ID: id("12"), Class: "virtual_machine", Stage: StageRetired, Attributes: map[string]any{"app": "crm"}},
		{ID: id("13"), Class: "server", Stage: StageInUse, Attributes: map[string]any{"app": "crm"}},
		{ID: id("14"), Class: "virtual_machine", Stage: StageInUse, Status: StatusMerged, Attributes: map[string]any{"app": "crm"}},
	}
	if m := g.Members(cis, classes); len(m) != 1 || m[0].ID != id("10") {
		t.Fatalf("members: %+v", m)
	}
	for name, in := range map[string]GroupInput{
		"bad key":      {Key: "CRM", Name: "x", Filter: Filter{NameHas: "a"}},
		"empty filter": {Key: "x", Name: "x"},
		"unknown":      {Key: "x", Name: "x", Filter: Filter{Classes: []string{"spaceship"}}},
		"stage":        {Key: "x", Name: "x", Filter: Filter{Stages: []Stage{"GONE"}}},
	} {
		if _, err := NewGroup(id("01"), id("02"), id("03"), id("04"), in, classes, now); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestAnAssetMovesThroughItsLifeAndIsDisposedOnlyWithACertificate(t *testing.T) {
	a := Asset{State: AssetRequested, Currency: "EUR", CostMinor: 129900}
	if err := a.Check(); err != nil {
		t.Fatal(err)
	}
	a, err := a.Move(AssetOrdered, nil)
	if err != nil || a.State.Stage() != StageOrdered {
		t.Fatalf("ordered: %v", err)
	}
	if _, err := a.Move(AssetDeployed, nil); err == nil {
		t.Fatal("an ordered asset was deployed before it arrived")
	}
	a, _ = a.Move(AssetInStock, nil)
	if _, err := a.Move(AssetDeployed, nil); err == nil {
		t.Fatal("deployed to nobody and nowhere")
	}
	a.AssignedTo = id("07")
	a, _ = a.Move(AssetDeployed, nil)
	a, _ = a.Move(AssetRetired, nil)
	if _, err := a.Move(AssetDisposed, &Disposal{Method: "WIPED"}); err == nil {
		t.Fatal("disposed without a certificate")
	}
	if _, err := a.Move(AssetDisposed, &Disposal{Method: "BURIED", Certificate: "C-1"}); err == nil {
		t.Fatal("disposed by an unknown method")
	}
	a, err = a.Move(AssetDisposed, &Disposal{Method: "DESTROYED", Certificate: " C-2026-17 ", By: id("07"), At: now})
	if err != nil || a.Disposal.Certificate != "C-2026-17" || a.State.Stage() != StageDisposed {
		t.Fatalf("disposed: %+v %v", a, err)
	}
	if _, err := a.Move(AssetInStock, nil); err == nil {
		t.Fatal("a disposed asset came back")
	}
	for name, bad := range map[string]Asset{
		"cost without currency": {State: AssetInStock, CostMinor: 1},
		"negative cost":         {State: AssetInStock, CostMinor: -1, Currency: "EUR"},
		"lower-case currency":   {State: AssetInStock, Currency: "eur"},
		"warranty before":       {State: AssetInStock, PurchasedOn: day(2), WarrantyUntil: day(1)},
		"unknown state":         {State: "LOST"},
	} {
		if bad.Check() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSoftwareIsGroupedByItsNormalisedProductAndAnAliasWins(t *testing.T) {
	classes := NewClasses(nil)
	for in, want := range map[string]string{
		"Mozilla Firefox 128.0 (x64)": "mozilla firefox", "  Visual  Studio Code ": "visual studio code",
		"7-Zip 23.01": "7-zip", "Notepad++ v8.6": "notepad++",
	} {
		if got := NormalProduct(in); got != want {
			t.Errorf("product %q: %q", in, got)
		}
	}
	if got := NormalPublisher("Microsoft Corporation"); got != "microsoft" {
		t.Errorf("publisher: %q", got)
	}
	alias, err := NewSoftwareAlias(id("01"), id("02"), id("03"), "", "Code", "Microsoft", "Visual Studio Code", now)
	if err != nil {
		t.Fatal(err)
	}
	cis := []CI{
		{ID: id("10"), Class: "software", Name: "Mozilla Firefox 128.0 (x64)", Attributes: map[string]any{"publisher": "Mozilla", "version": "128.0"}},
		{ID: id("11"), Class: "software", Name: "mozilla firefox", Attributes: map[string]any{"publisher": "Mozilla Corporation", "version": "115.1"}},
		{ID: id("12"), Class: "software", Name: "Code", Attributes: map[string]any{"publisher": "Microsoft Corporation", "version": "1.95"}},
		{ID: id("13"), Class: "server", Name: "db1"},
	}
	got := Normalise(cis, classes, []SoftwareAlias{alias}, map[shared.ID]int{id("10"): 3, id("11"): 1})
	if len(got) != 2 {
		t.Fatalf("products: %+v", got)
	}
	if got[0].Product != "Visual Studio Code" || !got[0].Aliased {
		t.Errorf("the alias: %+v", got[0])
	}
	ff := got[1]
	if ff.Product != "mozilla firefox" || ff.Publisher != "mozilla" || len(ff.Versions) != 2 || ff.Versions[0].Installations != 3 || ff.Versions[1].Installations != 1 {
		t.Errorf("firefox: %+v", ff)
	}
	if _, err := NewSoftwareAlias(id("01"), id("02"), id("03"), "", " ", "x", "y", now); err == nil {
		t.Error("an alias matching nothing")
	}
}
