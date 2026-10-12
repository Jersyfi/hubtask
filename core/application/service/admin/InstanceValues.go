// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"errors"
	"slices"
	"strings"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/quota"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The installation's level as a document: every key of every area with its declaration, read and
// written by kind (UC-INS-09 checks 1, 2 and 6).
//
// **One reading for three doors.** The screen and `hubctl` reach this through REST, the file
// through `InstanceLevelOf`, and both arrive here with the document as it was sent - so what a
// missing value means, and which values are refused, is decided once, here, for all three.

// The five areas, in the spelling the contract uses.
const (
	areaSignIn       = "sign_in"
	areaLegal        = "legal"
	areaLocalisation = "localisation"
	areaQuotas       = "quotas"
	areaAi           = "ai"
)

// instanceLevelOutput is the projection the control plane reads and writes back.
//
// **Every declared key, decided or not**, each with its kind. Answering only the decided ones would
// leave a screen unable to show that the others exist and are each workspace's; answering them
// without a kind would leave it guessing how to draw a key nobody decided yet. An unset entry
// carries no `value` at all rather than a zero somebody would read as a decision.
func instanceLevelOutput(level identityrepo.InstanceLevel) usecase.Output {
	signIn := usecase.Output{}
	for _, name := range identity.PolicySwitches() {
		declaration, held := identity.DeclarationOfSwitch(name)
		if !held {
			continue
		}
		value, decided := switchValueOf(level.Policy.Patch, name)
		place(signIn, string(name), entryOf(declaration, decided, value, level.Policy.Locks[name]))
	}

	legal := usecase.Output{}
	for _, link := range identity.LegalLinkNames() {
		value := level.Legal.Links.Of(link)
		place(legal, string(link), entryOf(identity.DeclarationOfLink(link), value != "", value, level.Legal.Locks[link]))
	}

	localisation := usecase.Output{}
	for _, name := range identity.LocalisationKeys() {
		declaration, _ := identity.DeclarationOfLocalisation(name)
		value := localisationValueOf(level.Localisation, name)
		place(localisation, name, entryOf(declaration, value != "", value, false))
	}

	quotas := usecase.Output{}
	for _, name := range quota.OperatorNames() {
		ceiling, decided := level.Quotas.Of(name)
		place(quotas, name, entryOf(quota.Declaration(), decided, ceiling, level.Quotas.Locked(name)))
	}

	ai := usecase.Output{}
	for _, name := range integration.AiSwitches() {
		declaration, held := integration.DeclarationOfAiSwitch(name)
		if !held {
			continue
		}
		value := aiValueOf(level.Ai, name)
		place(ai, string(name), entryOf(declaration, value != "", value, level.Ai.Locks[name]))
	}

	out := usecase.Output{
		areaSignIn:              signIn,
		areaLegal:               legal,
		areaLocalisation:        localisation,
		areaQuotas:              quotas,
		areaAi:                  ai,
		"source":                level.Source,
		"is_enforced_from_file": level.IsEnforcedFromFile,
	}
	if level.BlocklistFile != "" {
		out["blocklist_file"] = level.BlocklistFile
	}
	return out
}

// place puts an entry into its area with its position there: a JSON object has no order, and a
// screen draws the keys in the domain's order - the order the workspace's own screen uses.
func place(area usecase.Output, key string, entry usecase.Output) {
	entry["position"] = len(area)
	area[key] = entry
}

// entryOf is one key: decided or not, its value only where it was, its lock, and its declaration.
func entryOf(declaration identity.ValueDeclaration, decided bool, value any, locked bool) usecase.Output {
	entry := usecase.Output{"set": decided, "locked": locked, "kind": string(declaration.Kind)}
	if decided {
		entry["value"] = value
	}
	if len(declaration.Options) > 0 {
		options := make([]any, 0, len(declaration.Options))
		for _, option := range declaration.Options {
			options = append(options, option)
		}
		entry["options"] = options
	}
	if declaration.Minimum != nil {
		entry["minimum"] = *declaration.Minimum
	}
	if declaration.Maximum != nil {
		entry["maximum"] = *declaration.Maximum
	}
	if declaration.May != identity.MayNeverLock {
		entry["workspace_may"] = string(declaration.May)
	}
	return entry
}

// switchValueOf is one sign-in switch's value as the document carries it, and whether it is set.
//
//nolint:gocyclo,cyclop // one case per switch is the mapping
func switchValueOf(patch identity.PolicyPatch, name identity.PolicySwitch) (any, bool) {
	number := func(value *int) (any, bool) {
		if value == nil {
			return nil, false
		}
		return *value, true
	}
	flag := func(value *bool) (any, bool) {
		if value == nil {
			return nil, false
		}
		return *value, true
	}
	switch name {
	case identity.SwitchMinLength:
		return number(patch.MinLength)
	case identity.SwitchMinLowercase:
		return number(patch.MinLowercase)
	case identity.SwitchMinUppercase:
		return number(patch.MinUppercase)
	case identity.SwitchMinDigits:
		return number(patch.MinDigits)
	case identity.SwitchMinSymbols:
		return number(patch.MinSymbols)
	case identity.SwitchMinClasses:
		return number(patch.MinClasses)
	case identity.SwitchMaxRepeat:
		return number(patch.MaxRepeat)
	case identity.SwitchMaxAgeDays:
		return number(patch.MaxAgeDays)
	case identity.SwitchHistoryCount:
		return number(patch.HistoryCount)
	case identity.SwitchMinAgeHours:
		return number(patch.MinAgeHours)
	case identity.SwitchSessionMaxDays:
		return number(patch.SessionMaxDays)
	case identity.SwitchSessionIdleMinutes:
		return number(patch.SessionIdleMinutes)
	case identity.SwitchCommonPasswords:
		return flag(patch.CommonPasswords)
	case identity.SwitchContextWords:
		return flag(patch.ContextWords)
	case identity.SwitchBreachCheck:
		return flag(patch.BreachCheck)
	case identity.SwitchManagedAccounts:
		return flag(patch.ManagedAccounts)
	case identity.SwitchMfaRequiredFor:
		if patch.MfaRequiredFor == nil {
			return nil, false
		}
		return string(*patch.MfaRequiredFor), true
	case identity.SwitchMethods:
		if patch.Methods == nil {
			return nil, false
		}
		methods := make([]any, 0, len(*patch.Methods))
		for _, method := range *patch.Methods {
			methods = append(methods, method)
		}
		return methods, true
	}
	return nil, false
}

func localisationValueOf(defaults identity.LocalisationDefaults, name string) string {
	switch name {
	case identity.LocalisationLocale:
		return defaults.Locale
	case identity.LocalisationTimeZone:
		return defaults.TimeZone
	case identity.LocalisationWeekStart:
		return string(defaults.WeekStart)
	}
	return ""
}

// InstanceLevelOf is `instanceLevelFrom` for the one caller outside this package: the instance
// file, the third of §5's three doors, which goes through the same reading the other two do.
func InstanceLevelOf(document map[string]any) (identityrepo.InstanceLevel, error) {
	return instanceLevelFrom(usecase.Input(document))
}

// instanceLevelFrom reads the document a `PUT` carried.
//
// An unknown key is skipped rather than refused, for the reason the adapter skips an unknown row:
// a newer client talking to an older server should lose the key it does not have rather than the
// save. An entry without a value - absent, `null`, or an empty word - is a key the installation
// does not decide, in every area alike, and its lock goes with it: a lock on nothing is nothing.
func instanceLevelFrom(in usecase.Input) (identityrepo.InstanceLevel, error) {
	level := identityrepo.InstanceLevel{
		Policy: identity.PolicyLayer{Locks: map[identity.PolicySwitch]bool{}},
		Legal:  identity.LegalLayer{Locks: map[identity.LegalLink]bool{}},
		Quotas: identity.QuotaDefaults{Limits: map[string]int64{}, Locks: map[string]bool{}},
		Ai:     integration.AiLayer{Locks: map[integration.AiSwitch]bool{}},
	}

	readers := []struct {
		area    string
		declare func(name string) (identity.ValueDeclaration, bool)
		apply   func(name string, value any, locked bool) error
	}{
		{areaSignIn, func(name string) (identity.ValueDeclaration, bool) {
			return identity.DeclarationOfSwitch(identity.PolicySwitch(name))
		}, func(name string, value any, locked bool) error {
			applySwitch(&level.Policy.Patch, identity.PolicySwitch(name), value)
			if locked {
				level.Policy.Locks[identity.PolicySwitch(name)] = true
			}
			return nil
		}},
		{areaLegal, func(name string) (identity.ValueDeclaration, bool) {
			for _, link := range identity.LegalLinkNames() {
				if string(link) == name {
					return identity.DeclarationOfLink(link), true
				}
			}
			return identity.ValueDeclaration{}, false
		}, func(name string, value any, locked bool) error {
			link := identity.LegalLink(name)
			checked, err := identity.ValidLegalURL(link, wordOf(value))
			if err != nil {
				return atField(err, areaLegal, name)
			}
			level.Legal.Links = level.Legal.Links.With(link, checked)
			if locked {
				level.Legal.Locks[link] = true
			}
			return nil
		}},
		{areaLocalisation, identity.DeclarationOfLocalisation, func(name string, value any, locked bool) error {
			// Refused rather than ignored: "eine Instanz gibt einen Standard, nie ein Schloss"
			// (§5.7), and a `PUT` obeyed in part is one whose author believes something untrue.
			if locked {
				return shared.ErrValidation.
					WithDetail("admin.localisation_never_locked").
					WithParams(map[string]string{"setting": name})
			}
			return applyLocalisation(&level.Localisation, name, wordOf(value))
		}},
		{areaQuotas, func(name string) (identity.ValueDeclaration, bool) {
			for _, known := range quota.OperatorNames() {
				if known == name {
					return quota.Declaration(), true
				}
			}
			return identity.ValueDeclaration{}, false
		}, func(name string, value any, locked bool) error {
			level.Quotas.Limits[name] = int64(countOf(value))
			if locked {
				level.Quotas.Locks[name] = true
			}
			return nil
		}},
		{areaAi, func(name string) (identity.ValueDeclaration, bool) {
			return integration.DeclarationOfAiSwitch(integration.AiSwitch(name))
		}, func(name string, value any, locked bool) error {
			applyAiValue(&level.Ai, integration.AiSwitch(name), wordOf(value))
			if locked {
				level.Ai.Locks[integration.AiSwitch(name)] = true
			}
			return nil
		}},
	}

	for _, reader := range readers {
		sent, isObject := in[reader.area].(map[string]any)
		if !isObject {
			continue
		}
		for name, raw := range sent {
			declaration, known := reader.declare(name)
			if !known {
				continue
			}
			entry, isObject := raw.(map[string]any)
			if !isObject {
				return identityrepo.InstanceLevel{}, refusedValue(reader.area, name)
			}
			if err := settableAbove(reader.area, name, entry["value"]); err != nil {
				return identityrepo.InstanceLevel{}, err
			}
			value, decided, err := valueOfKind(reader.area, name, declaration, entry["value"])
			if err != nil {
				return identityrepo.InstanceLevel{}, err
			}
			if !decided {
				continue
			}
			locked, _ := entry["locked"].(bool)
			if err := reader.apply(name, value, locked); err != nil {
				return identityrepo.InstanceLevel{}, err
			}
		}
	}

	level.BlocklistFile = in.String("blocklist_file")
	return level, nil
}

// valueOfKind reads one value by its declared kind, and answers false for one that is not there.
//
// A `COUNT` is a whole number that is not negative - not a string of digits, not a fraction;
// past its bounds it is held at them on read, at every level, as a workspace's own is. A choice is
// one of its closed set.
//
//nolint:gocyclo,cyclop // one case per kind
func valueOfKind(area, name string, declaration identity.ValueDeclaration, raw any) (any, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	refused := refusedValue(area, name)
	switch declaration.Kind {
	case identity.KindCount:
		number, isNumber := intOfInput(raw)
		if !isNumber || number < 0 {
			return nil, false, refused
		}
		return number, true, nil
	case identity.KindFlag:
		flag, isFlag := raw.(bool)
		if !isFlag {
			return nil, false, refused
		}
		return flag, true, nil
	case identity.KindChoice:
		// The week start's ISO numbers are the spelling this key had before it shared the
		// account's words; the three that are a day of some level are still read as it.
		if day, isNumber := intOfInput(raw); isNumber && area == areaLocalisation {
			named, held := identity.WeekStartOfISO(day)
			if !held {
				return nil, false, refused
			}
			raw = string(named)
		}
		word, isWord := raw.(string)
		if !isWord || !slices.Contains(declaration.Options, word) {
			return nil, false, refused
		}
		return word, true, nil
	case identity.KindChoices:
		list, isList := raw.([]any)
		if !isList {
			return nil, false, refused
		}
		words := make([]string, 0, len(list))
		for _, each := range list {
			word, isWord := each.(string)
			if !isWord || !slices.Contains(declaration.Options, word) {
				return nil, false, refused
			}
			words = append(words, word)
		}
		return words, true, nil
	case identity.KindLink, identity.KindLocale, identity.KindTimeZone:
		word, isWord := raw.(string)
		if !isWord {
			return nil, false, refused
		}
		if strings.TrimSpace(word) == "" {
			return nil, false, nil
		}
		return word, true, nil
	}
	return nil, false, refused
}

//nolint:gocyclo,cyclop // one case per switch is the mapping
func applySwitch(patch *identity.PolicyPatch, name identity.PolicySwitch, value any) {
	number := func() *int {
		held := countOf(value)
		return &held
	}
	flag := func() *bool {
		held, _ := value.(bool)
		return &held
	}
	switch name {
	case identity.SwitchMinLength:
		patch.MinLength = number()
	case identity.SwitchMinLowercase:
		patch.MinLowercase = number()
	case identity.SwitchMinUppercase:
		patch.MinUppercase = number()
	case identity.SwitchMinDigits:
		patch.MinDigits = number()
	case identity.SwitchMinSymbols:
		patch.MinSymbols = number()
	case identity.SwitchMinClasses:
		patch.MinClasses = number()
	case identity.SwitchMaxRepeat:
		patch.MaxRepeat = number()
	case identity.SwitchMaxAgeDays:
		patch.MaxAgeDays = number()
	case identity.SwitchHistoryCount:
		patch.HistoryCount = number()
	case identity.SwitchMinAgeHours:
		patch.MinAgeHours = number()
	case identity.SwitchSessionMaxDays:
		patch.SessionMaxDays = number()
	case identity.SwitchSessionIdleMinutes:
		patch.SessionIdleMinutes = number()
	case identity.SwitchCommonPasswords:
		patch.CommonPasswords = flag()
	case identity.SwitchContextWords:
		patch.ContextWords = flag()
	case identity.SwitchBreachCheck:
		patch.BreachCheck = flag()
	case identity.SwitchManagedAccounts:
		patch.ManagedAccounts = flag()
	case identity.SwitchMfaRequiredFor:
		requirement := identity.MfaRequirement(wordOf(value))
		patch.MfaRequiredFor = &requirement
	case identity.SwitchMethods:
		methods, _ := value.([]string)
		patch.Methods = &methods
	}
}

// applyLocalisation takes a language and a zone only where a workspace could take them itself.
func applyLocalisation(defaults *identity.LocalisationDefaults, name, value string) error {
	switch name {
	case identity.LocalisationLocale:
		locale, err := identity.ValidDefaultLocale(value)
		if err != nil {
			return atField(err, areaLocalisation, name)
		}
		defaults.Locale = locale
	case identity.LocalisationTimeZone:
		zone, err := identity.ValidDefaultTimeZone(value)
		if err != nil {
			return atField(err, areaLocalisation, name)
		}
		defaults.TimeZone = zone
	case identity.LocalisationWeekStart:
		defaults.WeekStart = identity.WeekStart(value)
	}
	return nil
}

// settableAbove refuses, with its own reason, a value only a workspace may choose: the
// installation cannot name the legal basis a workspace that keeps nobody's content out of AI has
// to name (data-protection.md §4.1), so it holds a workspace at the opt-out or leaves it alone.
func settableAbove(area, name string, raw any) error {
	word, isWord := raw.(string)
	if area != areaAi || name != string(integration.AiSwitchPersonOptOut) || !isWord || word == "" {
		return nil
	}
	if position := integration.AiPersonOptOut(word); position.Valid() {
		return position.SettableAbove()
	}
	return nil
}

func applyAiValue(layer *integration.AiLayer, name integration.AiSwitch, value string) {
	switch name {
	case integration.AiSwitchPersonOptOut:
		position := integration.AiPersonOptOut(value)
		layer.PersonOptOut = &position
	case integration.AiSwitchSources:
		sources := integration.AiSources(value)
		layer.Sources = &sources
	case integration.AiSwitchMinJurisdiction:
		floor := integration.AiMinJurisdiction(value)
		layer.MinJurisdiction = &floor
	}
}

// intOfInput reads a whole number out of a decoded JSON document, which carries one as a float.
func intOfInput(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case float64:
		return int(number), number == float64(int64(number))
	default:
		return 0, false
	}
}

// wordOf and countOf read a value `valueOfKind` has already held to its kind.
func wordOf(value any) string {
	word, _ := value.(string)
	return word
}

func countOf(value any) int {
	number, _ := value.(int)
	return number
}

// refusedValue names the key in its own area, so a refusal lands at the row it is about.
func refusedValue(area, name string) error {
	return shared.ErrValidation.
		WithDetail("auth.policy_value_invalid").
		WithParams(map[string]string{"switch": area + "." + name}).
		WithFields(shared.FieldError{Path: "/" + area + "/" + name, Code: "auth.policy_value_invalid"})
}

// atField points a refusal the domain built for a workspace's own field at the installation's key
// instead: the code is the same, the field it is about is not.
func atField(err error, area, name string) error {
	var refusal *shared.Error
	if !errors.As(err, &refusal) {
		return err
	}
	code := refusal.DetailCode
	if code == "" {
		code = refusal.Code
	}
	return refusal.WithFields(shared.FieldError{Path: "/" + area + "/" + name, Code: code})
}
