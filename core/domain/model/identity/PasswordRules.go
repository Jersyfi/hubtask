// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// What a password must meet, and which of the rules it does not (ADR-0068 §7).
//
// **Two implementations of one rule, on purpose.** The client predicts at every keystroke what
// this decides on send, because a screen that says "at least twelve characters" where the
// workspace demands fifteen is a screen that lies. The two cannot be allowed to drift, so
// `api/fixtures/password-rules.json` carries rules, passwords and the violations they produce,
// and both sides read it - the same answer `capability.ts` gives to the same problem.
//
// **A rule is a code, never a sentence.** Each violation carries the message code the catalogue
// holds and the parameters it takes; the renderer turns that into a sentence in the reader's
// language (ADR-0011, rule 8). The same codes travel in `field_errors[]` when a password is
// refused, so one fact has one sentence whether the client saw the refusal coming or the server
// sent it.
//
// **NFKC before anything is counted.** The same word typed on two keyboards has to be one
// password - and it is the same normalisation applied before hashing, so what was counted is what
// is stored.

// PasswordRuleID names one rule. The spelling is the contract's: it is what
// `POST /auth/password:check` answers, what the field error's code ends in, and what the client
// keys its lines on.
type PasswordRuleID string

// The rules a client can answer itself: arithmetic over the characters.
const (
	RuleMinLength    PasswordRuleID = "min_length"
	RuleMinLowercase PasswordRuleID = "min_lowercase"
	RuleMinUppercase PasswordRuleID = "min_uppercase"
	RuleMinDigits    PasswordRuleID = "min_digits"
	RuleMinSymbols   PasswordRuleID = "min_symbols"
	RuleMinClasses   PasswordRuleID = "min_classes"
	RuleMaxRepeat    PasswordRuleID = "max_repeat"
	RuleContextWords PasswordRuleID = "context_words"
)

// The rules only the server can answer, because only it holds what they compare against.
const (
	// RuleCommon covers the embedded list and the operator's file both. Which of the two a
	// password was found in is not a reader's business, and saying would tell a guesser which
	// corpus to avoid.
	RuleCommon PasswordRuleID = "common"
	// RuleBreach is the corpus an installation configured, where one is.
	RuleBreach PasswordRuleID = "breach"
	// RuleHistory is the last n passwords of this account.
	RuleHistory PasswordRuleID = "history"
	// RuleNotCurrent is the one in force - a rule of its own, because "you already have this
	// one" is a different sentence from "you used it a while ago".
	RuleNotCurrent PasswordRuleID = "not_current"
)

// PasswordRuleCode is the catalogue code one rule's sentence is held under.
func (r PasswordRuleID) PasswordRuleCode() string { return "auth.password_rule." + string(r) }

// PasswordViolation is one rule a candidate does not meet, with the parameters its sentence
// takes. The parameter names are the catalogue's - `minimum` for the length, `count` for the
// rest - and a mismatch is a sentence with a hole in it, which is why the gate checks them.
type PasswordViolation struct {
	Rule   PasswordRuleID
	Params map[string]string
}

// PasswordContext is who is setting the password and where, for the rules that are about them
// rather than about the string.
type PasswordContext struct {
	Email         string
	DisplayName   string
	WorkspaceName string
	WorkspaceHost string
}

// NormalisePassword brings a candidate to the form everything else works on.
//
// A nil normaliser answers the input unchanged. That is for `CheckPassword`, the compatibility
// shim below, which has no way to be handed one - every other caller passes the port, and an
// ASCII password is byte-identical either way.
func NormalisePassword(form text.Normalizer, password string) string {
	if form == nil {
		return password
	}
	return form.NFKC(password)
}

// leetFolds are the substitutions a list lookup undoes: `P@ssw0rd` is `password` to everybody
// except a naive comparison, which is what a blocklist that missed it would be.
var leetFolds = strings.NewReplacer(
	"0", "o", "1", "l", "3", "e", "4", "a", "5", "s", "7", "t",
	"@", "a", "$", "s", "!", "i",
)

// FlattenPassword is the letters a list lookup compares: normalised, lower-cased, de-leeted.
func FlattenPassword(form text.Normalizer, value string) string {
	return leetFolds.Replace(strings.ToLower(NormalisePassword(form, value)))
}

// PasswordClasses counts the four kinds by Unicode category, so that `Ω` is an uppercase letter
// and an emoji is "other". The categories are named exactly - Ll, Lu, Nd - because that is what
// the client's `\p{Ll}` matches, and the two counts have to agree character for character.
type PasswordClasses struct {
	Lowercase int
	Uppercase int
	Digits    int
	// Symbols is everything else: punctuation, marks, an emoji, and a letter of a script with
	// no case. The switch that demands lowercase and uppercase carries the sentence saying a
	// script without letter case can never satisfy it.
	Symbols int
	// Classes is how many of the four appear at all - "n of four", which cannot be expressed in
	// the four counts.
	Classes int
}

// ClassesOf counts a candidate's characters.
func ClassesOf(form text.Normalizer, password string) PasswordClasses {
	var counted PasswordClasses
	for _, character := range NormalisePassword(form, password) {
		switch {
		case unicode.Is(unicode.Ll, character):
			counted.Lowercase++
		case unicode.Is(unicode.Lu, character):
			counted.Uppercase++
		case unicode.Is(unicode.Nd, character):
			counted.Digits++
		default:
			counted.Symbols++
		}
	}
	for _, count := range []int{counted.Lowercase, counted.Uppercase, counted.Digits, counted.Symbols} {
		if count > 0 {
			counted.Classes++
		}
	}
	return counted
}

// LongestRun is the longest run of one character, counted in code points so that an emoji is one.
func LongestRun(form text.Normalizer, password string) int {
	longest, run := 0, 0
	var previous rune
	for index, character := range NormalisePassword(form, password) {
		if index > 0 && character == previous {
			run++
		} else {
			run = 1
		}
		previous = character
		if run > longest {
			longest = run
		}
	}
	return longest
}

// ContextWords are the words this password may not carry: the product's own name, the address and
// its local part, the words of the person's name and the workspace's, and the host's first label.
//
// Short fragments are dropped - a three-letter name is in half of all passwords by accident - and
// every word is flattened, because the rule is about what a password *is*, not how it is spelled.
func ContextWords(form text.Normalizer, context PasswordContext) []string {
	words := []string{"hubtask"}
	if local, _, found := strings.Cut(context.Email, "@"); found && utf8.RuneCountInString(local) >= 3 {
		words = append(words, local)
	}
	if context.Email != "" {
		words = append(words, context.Email)
	}
	for _, source := range []string{context.DisplayName, context.WorkspaceName} {
		for _, part := range strings.Fields(source) {
			if utf8.RuneCountInString(part) >= 4 {
				words = append(words, part)
			}
		}
	}
	if label, _, _ := strings.Cut(context.WorkspaceHost, "."); utf8.RuneCountInString(label) >= 4 {
		words = append(words, label)
	}

	kept := make([]string, 0, len(words))
	for _, word := range words {
		flat := FlattenPassword(form, word)
		if utf8.RuneCountInString(flat) >= 3 {
			kept = append(kept, flat)
		}
	}
	return kept
}

// CarriesContextWord reports whether the candidate contains one of them.
func CarriesContextWord(form text.Normalizer, password string, context PasswordContext) bool {
	flat := FlattenPassword(form, password)
	for _, word := range ContextWords(form, context) {
		if strings.Contains(flat, word) {
			return true
		}
	}
	return false
}

// LocalPasswordViolations answers the rules a candidate fails that need nothing but the candidate
// itself - the ones a client predicts at every keystroke.
//
// In the order a screen draws them, because the answer is also what orders the list: the local
// rules first, and the server's as a block behind them, so a reader can see where they are.
func LocalPasswordViolations(
	policy PasswordPolicy, form text.Normalizer, password string, context PasswordContext,
) []PasswordViolation {
	normalised := NormalisePassword(form, password)
	counted := ClassesOf(form, normalised)
	violations := make([]PasswordViolation, 0, 8)

	minimum := policy.MinLength
	if minimum <= 0 {
		minimum = MinPasswordLength
	}
	if utf8.RuneCountInString(normalised) < minimum {
		violations = append(violations, PasswordViolation{
			Rule: RuleMinLength, Params: map[string]string{"minimum": itoa(minimum)},
		})
	}
	for _, rule := range []struct {
		id       PasswordRuleID
		required int
		counted  int
	}{
		{RuleMinLowercase, policy.MinLowercase, counted.Lowercase},
		{RuleMinUppercase, policy.MinUppercase, counted.Uppercase},
		{RuleMinDigits, policy.MinDigits, counted.Digits},
		{RuleMinSymbols, policy.MinSymbols, counted.Symbols},
		{RuleMinClasses, policy.MinClasses, counted.Classes},
	} {
		if rule.required > 0 && rule.counted < rule.required {
			violations = append(violations, PasswordViolation{
				Rule: rule.id, Params: map[string]string{"count": itoa(rule.required)},
			})
		}
	}
	if policy.MaxRepeat > 0 && LongestRun(form, normalised) > policy.MaxRepeat {
		violations = append(violations, PasswordViolation{
			Rule: RuleMaxRepeat, Params: map[string]string{"count": itoa(policy.MaxRepeat)},
		})
	}
	if policy.ContextWords && CarriesContextWord(form, normalised, context) {
		violations = append(violations, PasswordViolation{Rule: RuleContextWords})
	}
	return violations
}

// HistoryViolation is the parameterised form of the history rule, built where the comparison
// happens so that the count in the sentence is the count that was applied.
func HistoryViolation(count int) PasswordViolation {
	return PasswordViolation{Rule: RuleHistory, Params: map[string]string{"count": itoa(count)}}
}

// PasswordRefused is the one refusal every door builds: one detail for the banner the client
// deliberately does not draw, and one field error per violated rule, each carrying its own code
// and parameters.
//
// The path is `/password` for every one of them. A rule is not a field of its own: the field is
// the password, and the list under it is what says which rule refused - which is decision 7 of
// the milestone, and SC 3.3.1's answer as well.
func PasswordRefused(violations []PasswordViolation) error {
	if len(violations) == 0 {
		return nil
	}
	fields := make([]shared.FieldError, 0, len(violations))
	for _, violation := range violations {
		fields = append(fields, shared.FieldError{
			Path:   "/password",
			Code:   violation.Rule.PasswordRuleCode(),
			Params: violation.Params,
		})
	}
	return shared.ErrValidation.
		WithDetail("auth.password_refused").
		WithParams(map[string]string{"rules": itoa(len(violations))}).
		WithFields(fields...)
}

// CheckPasswordAgainst is the local half of the rule as a refusal: what a caller with no lists,
// no history and no corpus can decide on its own.
//
// The ceiling comes first and alone, because it is not a policy switch: a paste of a whole
// document is a validation error rather than a hashing bill, and reporting it beside "at least
// twelve characters" would be a list with a nonsense line in it.
func CheckPasswordAgainst(
	policy PasswordPolicy, form text.Normalizer, password string, context PasswordContext,
) error {
	if utf8.RuneCountInString(NormalisePassword(form, password)) > MaxPasswordLength {
		return shared.ErrValidation.
			WithDetail("auth.password_too_long").
			WithParams(map[string]string{"maximum": itoa(MaxPasswordLength)}).
			WithFields(shared.FieldError{Path: "/password", Code: "auth.password_too_long"})
	}
	return PasswordRefused(LocalPasswordViolations(policy, form, password, context))
}
