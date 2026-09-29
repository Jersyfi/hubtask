// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// InstanceSettingRepository is the installation's own level (ADR-0070 §2).
//
// **This file is the only thing that knows how a switch is spelled in a row.** The keys are dotted
// and the values are JSON, because the set grows with every feature that has an installation-wide
// default and a column per switch would be a migration per preference; on the other side of this
// seam the level is the typed value the domain resolves, and nothing above here parses anything.
// It is `settingsDocument`'s discipline applied one level up.
type InstanceSettingRepository struct {
	// source is what a read reports as the origin of the values in force: "database", or the path
	// of the file that wrote them (ADR-0070 §5). One source per mode, never two.
	source string
	// enforced is the `enforce` mode: the file is rewritten at every start, so the writing routes
	// refuse rather than accept a change the next restart would silently undo.
	enforced bool
}

func NewInstanceSettingRepository() InstanceSettingRepository {
	return InstanceSettingRepository{source: "database"}
}

// FromFile is the repository that knows a file is in play.
//
// `seed` leaves this a plain database repository once the seeding is done — the file wrote the
// values once and the API owns them from then on, which is what "schreibt sie beim ersten Start und
// lässt sie danach in Ruhe" means. `enforce` keeps both facts, because both are answers a reader
// needs: where the values came from, and why a save is refused.
func FromFile(path string, enforced bool) InstanceSettingRepository {
	if path == "" {
		return NewInstanceSettingRepository()
	}
	if !enforced {
		return InstanceSettingRepository{source: "database"}
	}
	return InstanceSettingRepository{source: path, enforced: true}
}

var _ repository.InstanceSettings = InstanceSettingRepository{}

// The two areas this build manages. A `PUT` empties what it did not decide inside these and
// touches nothing outside them, so an area added later is not cleared by a write to this one.
const (
	signInArea = "sign_in"
	legalArea  = "legal"
	// localisationArea holds the three defaults a workspace inherits. No lock is written here,
	// ever: the concept forbids one, and the type has nowhere to put it.
	localisationArea = "localisation"
	// quotaArea holds one ceiling per quota, keyed by the quota's own name.
	quotaArea = "quota"
)

// The keys, as one list rather than as literals in a read and a write that could disagree.
const (
	keyMinLength          = signInArea + ".min_length"
	keyMinLowercase       = signInArea + ".min_lowercase"
	keyMinUppercase       = signInArea + ".min_uppercase"
	keyMinDigits          = signInArea + ".min_digits"
	keyMinSymbols         = signInArea + ".min_symbols"
	keyMinClasses         = signInArea + ".min_classes"
	keyMaxRepeat          = signInArea + ".max_repeat"
	keyCommonPasswords    = signInArea + ".common_passwords"
	keyContextWords       = signInArea + ".context_words"
	keyBreachCheck        = signInArea + ".breach_check"
	keyMaxAgeDays         = signInArea + ".max_age_days"
	keyHistoryCount       = signInArea + ".history_count"
	keyMinAgeHours        = signInArea + ".min_age_hours"
	keyMfaRequiredFor     = signInArea + ".mfa_required_for"
	keyMethods            = signInArea + ".methods"
	keySessionMaxDays     = signInArea + ".session_max_days"
	keySessionIdleMinutes = signInArea + ".session_idle_minutes"
	keyBlocklistFile      = signInArea + ".blocklist_file"
)

// switchKeys maps each policy switch to its row's key. `rotation_from` is deliberately absent: it
// is an event a workspace raises for its own people, and an operator who wanted every account on
// the installation to change its password would be asking for a different feature with a different
// blast radius (ADR-0068 §3).
var switchKeys = map[identity.PolicySwitch]string{
	identity.SwitchMinLength:          keyMinLength,
	identity.SwitchMinLowercase:       keyMinLowercase,
	identity.SwitchMinUppercase:       keyMinUppercase,
	identity.SwitchMinDigits:          keyMinDigits,
	identity.SwitchMinSymbols:         keyMinSymbols,
	identity.SwitchMinClasses:         keyMinClasses,
	identity.SwitchMaxRepeat:          keyMaxRepeat,
	identity.SwitchCommonPasswords:    keyCommonPasswords,
	identity.SwitchContextWords:       keyContextWords,
	identity.SwitchBreachCheck:        keyBreachCheck,
	identity.SwitchMaxAgeDays:         keyMaxAgeDays,
	identity.SwitchHistoryCount:       keyHistoryCount,
	identity.SwitchMinAgeHours:        keyMinAgeHours,
	identity.SwitchMfaRequiredFor:     keyMfaRequiredFor,
	identity.SwitchMethods:            keyMethods,
	identity.SwitchSessionMaxDays:     keySessionMaxDays,
	identity.SwitchSessionIdleMinutes: keySessionIdleMinutes,
}

// Read answers the whole level.
func (r InstanceSettingRepository) Read(ctx context.Context) (repository.InstanceLevel, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.InstanceLevel{}, err
	}

	rows, err := queries.ReadInstanceSettings(ctx)
	if err != nil {
		return repository.InstanceLevel{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the instance settings: %w", err))
	}

	level := repository.InstanceLevel{
		Policy: identity.PolicyLayer{Locks: map[identity.PolicySwitch]bool{}},
		Legal:  identity.LegalLayer{Locks: map[identity.LegalLink]bool{}},
		Quotas: identity.QuotaDefaults{
			Limits: map[string]int64{}, Locks: map[string]bool{},
		},
		Source:             r.source,
		IsEnforcedFromFile: r.enforced,
	}
	for _, row := range rows {
		locked := row.LockOrigin != "OPEN"
		if err := applyRow(&level, row.Key, row.Value, locked); err != nil {
			return repository.InstanceLevel{}, err
		}
	}
	return level, nil
}

// applyRow folds one row into the level. A key this build does not know is skipped rather than
// refused: a newer version may have written it, and an older one that refused to start over a
// setting it does not read would make a rolling update impossible (ADR-0003's expand/contract).
//
//nolint:gocyclo,cyclop // one case per key is the mapping; splitting it hides which key is which
func applyRow(level *repository.InstanceLevel, key string, raw []byte, locked bool) error {
	number := func() (*int, error) {
		var value int
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, malformedSetting(key, err)
		}
		return &value, nil
	}
	flag := func() (*bool, error) {
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, malformedSetting(key, err)
		}
		return &value, nil
	}
	word := func() (string, error) {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", malformedSetting(key, err)
		}
		return value, nil
	}

	patch := &level.Policy.Patch
	var err error
	switch key {
	case keyMinLength:
		patch.MinLength, err = number()
	case keyMinLowercase:
		patch.MinLowercase, err = number()
	case keyMinUppercase:
		patch.MinUppercase, err = number()
	case keyMinDigits:
		patch.MinDigits, err = number()
	case keyMinSymbols:
		patch.MinSymbols, err = number()
	case keyMinClasses:
		patch.MinClasses, err = number()
	case keyMaxRepeat:
		patch.MaxRepeat, err = number()
	case keyMaxAgeDays:
		patch.MaxAgeDays, err = number()
	case keyHistoryCount:
		patch.HistoryCount, err = number()
	case keyMinAgeHours:
		patch.MinAgeHours, err = number()
	case keySessionMaxDays:
		patch.SessionMaxDays, err = number()
	case keySessionIdleMinutes:
		patch.SessionIdleMinutes, err = number()
	case keyCommonPasswords:
		patch.CommonPasswords, err = flag()
	case keyContextWords:
		patch.ContextWords, err = flag()
	case keyBreachCheck:
		patch.BreachCheck, err = flag()
	case keyMfaRequiredFor:
		var value string
		if value, err = word(); err == nil {
			requirement := identity.MfaRequirement(value)
			patch.MfaRequiredFor = &requirement
		}
	case keyMethods:
		var methods []string
		if err = json.Unmarshal(raw, &methods); err != nil {
			return malformedSetting(key, err)
		}
		patch.Methods = &methods
	case keyBlocklistFile:
		level.BlocklistFile, err = word()
	case localisationArea + ".locale":
		level.Localisation.Locale, err = word()
	case localisationArea + ".time_zone":
		level.Localisation.TimeZone, err = word()
	case localisationArea + ".week_start":
		var start *int
		if start, err = number(); err == nil {
			level.Localisation.WeekStart = *start
		}
	default:
		if name, isQuota := strings.CutPrefix(key, quotaArea+"."); isQuota {
			var ceiling int64
			if err = json.Unmarshal(raw, &ceiling); err != nil {
				return malformedSetting(key, err)
			}
			level.Quotas.Limits[name] = ceiling
			if locked {
				level.Quotas.Locks[name] = true
			}
			return nil
		}
		if link, isLegal := legalLinkOf(key); isLegal {
			var value string
			if value, err = word(); err == nil {
				level.Legal.Links = level.Legal.Links.With(link, value)
				if locked {
					level.Legal.Locks[link] = true
				}
			}
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	if locked {
		if name, isSwitch := switchOf(key); isSwitch {
			level.Policy.Locks[name] = true
		}
	}
	return nil
}

func legalLinkOf(key string) (identity.LegalLink, bool) {
	for _, link := range identity.LegalLinkNames() {
		if key == legalArea+"."+string(link) {
			return link, true
		}
	}
	return "", false
}

func switchOf(key string) (identity.PolicySwitch, bool) {
	for name, candidate := range switchKeys {
		if candidate == key {
			return name, true
		}
	}
	return "", false
}

func malformedSetting(key string, cause error) error {
	// Fail closed, `RequireAdminTotp`'s reasoning: a value this build cannot read must not be
	// reported as a switch that is off.
	return shared.ErrInternal.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("the instance setting %q does not parse: %w", key, cause))
}

// Write replaces the level whole.
func (r InstanceSettingRepository) Write(
	ctx context.Context, level repository.InstanceLevel, by shared.ID, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	rows, err := rowsOf(level)
	if err != nil {
		return err
	}

	actor := pgtype.UUID{}
	if !by.IsZero() {
		parsed, err := uuidOf(by)
		if err != nil {
			return err
		}
		actor = parsed
	}
	moment := pgtype.Timestamptz{Time: at.UTC(), Valid: true}

	kept := make([]string, 0, len(rows))
	for _, row := range rows {
		kept = append(kept, row.key)
		if err := queries.PutInstanceSetting(ctx, sqlc.PutInstanceSettingParams{
			Key: row.key, Value: row.value, LockOrigin: row.lock,
			UpdatedBy: actor, UpdatedAt: moment,
		}); err != nil {
			return shared.ErrUnavailable.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("writing the instance setting %q: %w", row.key, err))
		}
	}

	if err := queries.DeleteInstanceSettingsExcept(ctx, sqlc.DeleteInstanceSettingsExceptParams{
		Areas: []string{signInArea, legalArea, localisationArea, quotaArea},
		Kept:  kept,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing the instance settings: %w", err))
	}
	return nil
}

// settingRow is one row on its way to the table: the key, the value as JSON, and the lock.
type settingRow struct {
	key   string
	value []byte
	lock  string
}

func rowsOf(level repository.InstanceLevel) ([]settingRow, error) {
	rows := make([]settingRow, 0, len(switchKeys)+5)

	lockOf := func(locked bool, origin identity.LockOrigin) string {
		if !locked {
			return "OPEN"
		}
		if origin == "" {
			return string(identity.LockInstance)
		}
		return string(origin)
	}

	patch := level.Policy.Patch
	values := map[identity.PolicySwitch]any{}
	if patch.MinLength != nil {
		values[identity.SwitchMinLength] = *patch.MinLength
	}
	if patch.MinLowercase != nil {
		values[identity.SwitchMinLowercase] = *patch.MinLowercase
	}
	if patch.MinUppercase != nil {
		values[identity.SwitchMinUppercase] = *patch.MinUppercase
	}
	if patch.MinDigits != nil {
		values[identity.SwitchMinDigits] = *patch.MinDigits
	}
	if patch.MinSymbols != nil {
		values[identity.SwitchMinSymbols] = *patch.MinSymbols
	}
	if patch.MinClasses != nil {
		values[identity.SwitchMinClasses] = *patch.MinClasses
	}
	if patch.MaxRepeat != nil {
		values[identity.SwitchMaxRepeat] = *patch.MaxRepeat
	}
	if patch.CommonPasswords != nil {
		values[identity.SwitchCommonPasswords] = *patch.CommonPasswords
	}
	if patch.ContextWords != nil {
		values[identity.SwitchContextWords] = *patch.ContextWords
	}
	if patch.BreachCheck != nil {
		values[identity.SwitchBreachCheck] = *patch.BreachCheck
	}
	if patch.MaxAgeDays != nil {
		values[identity.SwitchMaxAgeDays] = *patch.MaxAgeDays
	}
	if patch.HistoryCount != nil {
		values[identity.SwitchHistoryCount] = *patch.HistoryCount
	}
	if patch.MinAgeHours != nil {
		values[identity.SwitchMinAgeHours] = *patch.MinAgeHours
	}
	if patch.MfaRequiredFor != nil {
		values[identity.SwitchMfaRequiredFor] = string(*patch.MfaRequiredFor)
	}
	if patch.Methods != nil {
		values[identity.SwitchMethods] = *patch.Methods
	}
	if patch.SessionMaxDays != nil {
		values[identity.SwitchSessionMaxDays] = *patch.SessionMaxDays
	}
	if patch.SessionIdleMinutes != nil {
		values[identity.SwitchSessionIdleMinutes] = *patch.SessionIdleMinutes
	}

	// In the domain's own order, so that two writes of one level produce the same statements and a
	// diff of the trail reads top to bottom.
	for _, name := range identity.PolicySwitches() {
		value, decided := values[name]
		if !decided {
			continue
		}
		key, known := switchKeys[name]
		if !known {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, shared.Internalf("postgres: encoding the instance setting %q: %w", key, err)
		}
		rows = append(rows, settingRow{key: key, value: encoded, lock: lockOf(level.Policy.Locks[name], "")})
	}

	for _, link := range identity.LegalLinkNames() {
		value := level.Legal.Links.Of(link)
		if value == "" && !level.Legal.Locks[link] {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, shared.Internalf("postgres: encoding the instance setting %q: %w", link, err)
		}
		rows = append(rows, settingRow{
			key:   legalArea + "." + string(link),
			value: encoded,
			lock:  lockOf(level.Legal.Locks[link], ""),
		})
	}

	// The three localisation defaults, each written only where the installation decided it, and
	// each with `OPEN` — the concept's §5.7 says an instance gives a default and never a lock, and
	// writing the lock unconditionally is how that stays true however the level was assembled.
	for key, value := range map[string]string{
		localisationArea + ".locale":    level.Localisation.Locale,
		localisationArea + ".time_zone": level.Localisation.TimeZone,
	} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, shared.Internalf("postgres: encoding the instance setting %q: %w", key, err)
		}
		rows = append(rows, settingRow{key: key, value: encoded, lock: "OPEN"})
	}
	if level.Localisation.WeekStart != 0 {
		encoded, err := json.Marshal(level.Localisation.WeekStart)
		if err != nil {
			return nil, shared.Internalf("postgres: encoding the week start: %w", err)
		}
		rows = append(rows, settingRow{
			key: localisationArea + ".week_start", value: encoded, lock: "OPEN",
		})
	}

	// One row per quota, because the lock is per quota: an operator may fix one ceiling and leave
	// the others to each workspace.
	for name, ceiling := range level.Quotas.Limits {
		encoded, err := json.Marshal(ceiling)
		if err != nil {
			return nil, shared.Internalf("postgres: encoding the quota default %q: %w", name, err)
		}
		rows = append(rows, settingRow{
			key:   quotaArea + "." + name,
			value: encoded,
			lock:  lockOf(level.Quotas.Locks[name], identity.LockInstance),
		})
	}

	if level.BlocklistFile != "" {
		encoded, err := json.Marshal(level.BlocklistFile)
		if err != nil {
			return nil, shared.Internalf("postgres: encoding the blocklist path: %w", err)
		}
		rows = append(rows, settingRow{key: keyBlocklistFile, value: encoded, lock: "OPEN"})
	}

	return rows, nil
}
