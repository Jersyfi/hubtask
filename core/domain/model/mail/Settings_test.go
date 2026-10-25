// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestTheLevelsAdmitWhomTheySay(t *testing.T) {
	lists := Senders{Allowed: []string{"@lieferant.de", "chef@steuer.de"}, Blocked: []string{"@spam.example"}}
	cases := []struct {
		level  Level
		sender string
		member bool
		want   bool
	}{
		{LevelMembers, "anna@gmail.com", true, true},
		{LevelMembers, "fremd@gmail.com", false, false},
		{LevelAllowed, "a@eu.lieferant.de", false, true},
		{LevelAllowed, "Chef@Steuer.de", false, true},
		{LevelAllowed, "x@steuer.de", false, false},
		{LevelAnyone, "fremd@gmail.com", false, true},
		{LevelAnyone, "x@spam.example", false, false},
		{LevelMembers, "member@spam.example", true, false},
	}
	for _, c := range cases {
		senders := lists
		senders.Level = c.level
		if got := senders.Admits(c.sender, c.member); got != c.want {
			t.Errorf("%s admits %s (member %v) = %v, want %v", c.level, c.sender, c.member, got, c.want)
		}
	}
}

func TestTheMostSpecificSenderRouteWins(t *testing.T) {
	office, shop, accounting := shared.ID("office"), shared.ID("shop"), shared.ID("accounting")
	routes := []SenderRoute{
		{Match: "@lieferant.de", CollectionID: shop},
		{Match: "@eu.lieferant.de", CollectionID: office},
		{Match: "rechnung@eu.lieferant.de", CollectionID: accounting},
	}
	for sender, want := range map[string]shared.ID{
		"a@lieferant.de":           shop,
		"a@eu.lieferant.de":        office,
		"rechnung@eu.lieferant.de": accounting,
	} {
		if got, ok := RouteBySender(routes, sender); !ok || got != want {
			t.Errorf("RouteBySender(%s) = %s, %v; want %s", sender, got, ok, want)
		}
	}
	if _, ok := RouteBySender(routes, "a@other.de"); ok {
		t.Error("no rule names other.de")
	}
}

func TestSettingsAreNormalised(t *testing.T) {
	settings, err := Settings{
		Name: " Familie-Meier ",
		Senders: Senders{
			Allowed: []string{"Lieferant.de", "@lieferant.de", "Chef@Steuer.de"},
			Blocked: []string{"@spam.example"},
		},
		SenderRoutes: []SenderRoute{{Match: "@lieferant.de", CollectionID: "shop"}},
	}.Normalised()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Name != "familie-meier" || settings.Senders.Level != LevelMembers {
		t.Fatalf("got %+v", settings)
	}
	if want := []string{"@lieferant.de", "chef@steuer.de"}; !reflect.DeepEqual(settings.Senders.Allowed, want) {
		t.Fatalf("allowed %v, want %v", settings.Senders.Allowed, want)
	}
}

func TestBadSettingsNameTheirFields(t *testing.T) {
	_, err := Settings{
		Name:         "-bad-",
		Senders:      Senders{Level: "SOME", Allowed: []string{"not an address"}},
		SenderRoutes: []SenderRoute{{Match: "@x.de"}, {Match: "nodot", CollectionID: "c"}},
	}.Normalised()
	var failure *shared.Error
	if !errors.As(err, &failure) || !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("want a validation error, got %v", err)
	}
	paths := map[string]bool{}
	for _, field := range failure.Fields {
		paths[field.Path] = true
	}
	for _, path := range []string{"/name", "/senders/level", "/senders/allowed/0", "/sender_routes/0/collection_id", "/sender_routes/1/match"} {
		if !paths[path] {
			t.Errorf("no error at %s in %v", path, failure.Fields)
		}
	}
}

func TestANewWorkspaceHasItsIntakeOff(t *testing.T) {
	if settings := DefaultSettings(); settings.Enabled || settings.Senders.Level != LevelMembers {
		t.Fatalf("got %+v", settings)
	}
}
