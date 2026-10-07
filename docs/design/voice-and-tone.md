# Voice and Tone — the writing rules

The counterpart to [`design-system.md`](./design-system.md) for words

---

## 0. What this is

The rules a component author — or an assistant writing a pull request — applies without asking
anybody, each stated so that a reviewer can point at a number and say a label breaks it.

It covers the places a component puts words: **buttons**, **errors**, **empty states** and
**proposals** (§7). It does not cover documentation, the website, or commit messages.

**Every string here is a catalogue entry, not a sentence in a component**
([ADR-0011](../adr/ADR-0011-i18n-message-codes.md),
[`i18n-l10n.md`](../architecture/i18n-l10n.md) §1): the server emits a code plus parameters and
never a finished sentence, and the client renders it from `locales/en.json`. A component that
hard-codes a sentence breaks this page and the product's translatability at once.

**English is the source language, not the only one.** A sentence that only works because of English
word order, or reads as a joke in English and as nonsense elsewhere, cannot be translated.

---

## 1. Case, punctuation, person

**1.1 Sentence case. Everywhere.** Buttons, headings, labels, menu items, dialog titles, column
headers, tab labels. `Create task`, never `Create Task`. Capitalise only the first word and what
would be capitalised mid-sentence — a product name, a person's name, `PostgreSQL`, `Europe/Berlin`.

**1.2 A full stop ends a sentence and nothing else.** Body copy, error messages and helper text
are sentences and take one. Button labels, headings, menu items, column headers, chips, tooltips of
one fragment, and email subject lines are not sentences and take none.

**1.3 A string that ends in an identifier ends without a full stop.** `Reference: {request_id}` —
a stop looks like part of the identifier once copied into a support ticket. The one exception to
1.2.

**1.4 Second person, singular, present tense.** "You do not have permission for this action."
Never "the user", never a passive that hides who does what. A translation keeps the same register
through its whole file ([`i18n-l10n.md`](../architecture/i18n-l10n.md) §3); in German that is the
informal *du*.

**1.5 "We" appears only where Hubtask owns a failure.** `Something went wrong on our side` is the
sentence "we" exists for. Everywhere else the sentence is about the reader's work, not about us.

**1.6 No exclamation marks doing the enthusiasm's work** (`design-system.md` §8). A celebration is
carried by the motion the design system defines (§7), not by punctuation.

**1.7 No "please" as a reflex.** It earns its place in exactly two cases: **Hubtask is refusing
something the reader is entitled to** ("Too many requests. Please try again in a moment.") and
**Hubtask is asking for work it caused** ("Please try again in a moment" after a dependency
failed). Asking somebody to sign in is neither.

---

## 2. Buttons

**2.1 The label is the verb of what happens.** `Create task`, `Archive`, `Send invitation`. Not
`OK`, not `Submit`, not `Yes`. A person reading only the button must be able to say what it will
do.

**2.2 The verb and its object, where the object is not obvious from context.** `Delete` inside a
task's own menu; `Delete work package` in a confirmation that could be about any of five levels.

**2.3 The wording stays consistent through the whole flow.** What reads `Publish` reports
`Published` afterwards and appears as `Published` in the activity stream.

**2.4 While it is working, the label becomes the present participle of its own verb.** `Create
task` → `Creating…`, `Send invitation` → `Sending…`. Not `Please wait`, not a label that vanishes:
the button keeps its width and its place, because rule 6 forbids animating layout.

**2.5 The cancelling button says what it does, not what it is.** `Cancel` is right when nothing has
happened yet. `Discard changes` is right when something would be lost — and then the destructive
button, not the safe one, carries the specific verb.

**2.6 A destructive button names what is destroyed.** `Delete permanently` where deletion is not
recoverable; the object where several are in reach.

---

## 3. Errors

**3.1 An error names the fix, not only the fault** — fault, consequence, fix, in that order, and the
fix is something the reader can actually do:

> `crypto.no_encryption_key` — "This installation has no encryption key configured, so it cannot
> store that securely. Set HUBTASK_ENCRYPTION_KEYS."

**3.2 Where there is no fix, say so and stop.** `errors.not_found` — "This entry does not exist."
is finished; an invented instruction is worse than none.

**3.3 Never blame the reader, and never blame them by grammar either.** "You entered an invalid
date" and "The date could not be read" describe the same event; use the second. Reserve "you" in an
error for permissions and for what the reader may do about it.

**3.4 The reader's vocabulary, not the system's.** `request`, `payload`, `entity`, `cursor`,
`null`, `parameter` and `serialisation` are how the code thinks. A message that reaches a person in
the interface uses the nouns the interface uses: entry, workspace, collection, task. A code that
only ever appears in an API answer, read by a developer through a problem document, may use API
vocabulary; a code that can reach a person — the generic `errors.*` fallbacks can — uses the
reader's.

**3.5 An error says what is still true.** "It stays unassigned", "Nothing is being delivered to it
until somebody switches it back on" — what a person needs to decide whether to act now.

**3.6 A recoverable failure says when to try again**, and says it in the message rather than only
in a header: "Please try again in {retry_after_seconds} seconds."

**3.7 An internal failure shows its reference where the reader can copy it.** The `request_id`
stands beside the message, ending without a full stop (1.3).

**3.8 A refusal is said once, where it is about.** A field's refusal is said at the field, not
repeated in a banner above the form; how a refused field is drawn and announced is
[`design-system.md`](./design-system.md) §10, 3.3.1.

---

## 4. Empty states

An empty list has three causes, and one sentence cannot serve all three.

**4.1 Empty because nothing has been made yet.** Say what this place is for and offer the one
action that fills it. This is the only empty state that carries a call to action.

> No tasks in this collection yet. — `Create task`

**4.2 Empty because a filter or a search excluded everything.** Say that the filter did it, and
offer to widen it. Never the same copy as 4.1.

> No task matches these filters. — `Clear filters`

**4.3 Empty because the emptiness is the good outcome.** An inbox with nothing in it, a rule with
no failures, a queue that has drained. State the fact plainly, offer nothing, and do not celebrate
it — §7's celebrations mark work completed, and an empty list is not an achievement.

> Nothing waiting.

**4.4 Empty because something failed** is not an empty state. It is an error, and it goes through
§3 with the retry that belongs to it. A failure rendered as "no results" is a lie the reader acts
on.

**4.5 No empty state apologises.** "Sorry, nothing here" is neither true nor useful.

---

## 5. Length

`design-system.md` rule 4: everything grows by 40 %.

**5.1 A button label is one or two words wherever the language allows.** `Send invitation` is
already `Einladung versenden`.

**5.2 An error is one sentence, or two short ones.** Fault and fix. A third sentence is
documentation, and documentation belongs behind a link.

**5.3 Nothing is written to a measured width.** A string tuned to fit one line in English wraps
badly in every other language.

---

## 6. Ten codes, checked

The rules applied to ten entries of `locales/en.json` as the catalogue holds them. Fixing what
disagrees is optional; hiding it is not.

| Code | Rules | Verdict |
|---|---|---|
| `bulk.no_operations` | 3.1, 3.4 | **Model entry.** Fix, consequence, the reader's vocabulary. |
| `crypto.no_encryption_key` | 3.1 | **Model entry.** Fault, consequence, a fix specific enough to act on. |
| `backup.no_parent_archive` | 3.1, 3.5 | **Model entry.** Why, what is still true, the action. |
| `automation.action_not_available_yet` | 3.1, 3.3 | **Passes.** A missing capability, not the reader's error. |
| `errors.not_found` | 3.2 | **Passes.** No fix, and none invented. |
| `errors.rate_limited` | 1.7, 3.6 | **Passes.** "Please" earned: Hubtask refuses something the reader may do. |
| `errors.internal` | 1.3, 1.5 | **Passes.** "Our side" owns the failure; the reference ends without a full stop. |
| `errors.unauthenticated` | 1.7 | **Disagrees.** "Please sign in." — neither a refusal nor our fault. `Sign in.` |
| `errors.validation_failed` | 3.1, 3.4 | **Disagrees.** "The request contains invalid values." names neither the fix nor the value, in API vocabulary. The field-level codes beneath it do both, so the finding is this fallback reaching a person at all. |
| `errors.conflict` | 3.1, 3.4 | **Disagrees.** "This action conflicts with the current state." is the system describing itself; `errors.version_conflict` ("Someone else changed this entry in the meantime.") is actionable. |

Two more that break a rule:

* `webhooks.target_rate_limited` — "The target asked us to slow down." Breaks 1.5: no failure of
  ours is owned.
* `items.auto_assign_no_candidate` — "No candidate of the assignment policy can receive this entry
  at the moment." Breaks 3.4: the model's vocabulary.

The catalogue uses no exclamation mark as punctuation (1.6) and no title case in a sentence (1.1).

---

## 7. A proposal

A suggestion the server made — a title, a tree of work, the labels an entry belongs under, a
summary, a translation, a template drafted from a description — is rendered through one
component, `AISuggestion`, and its words follow four more rules. The guardrails behind them are
[`ai-first.md`](../architecture/ai-first.md) §2.

**7.1 Offered, never asserted.** The heading names the kind and says that it is a proposal:
`Suggested title`, `Suggested breakdown`, `Suggested labels`, `Summary suggested` — and never
`Title`, which is what the entry's own field is called.

**7.2 It names its model where the reader can find it, and nowhere the eye lands first.** The
provenance — model, when, prompt version — is one line, collapsed by default, opened by a
control that says `Where this came from`. Not a badge in the heading, not a logo, not a colour
alone.

**7.3 Accepted in one gesture and dismissed in one.** Two buttons, §2's verbs: `Apply` (or the
verb of what accepting does — `Create work packages`, `Add labels`) and `Dismiss`. No
confirmation dialog for either: an accepted field can be changed back, and a dismissed proposal can
be asked for again. A `stale` proposal — the entry moved since it was made — says so in the
heading's line, `This entry changed since — ask again`, and offers the ask and the dismissal, not
the apply.

**7.4 It never counts, nudges or celebrates.** No "3 suggestions waiting", no "you have not
looked at this yet", and no celebration when one is accepted: `design-system.md` §7 marks what the
person did, and a field the model proposed is not the person's work. A proposal that is still being
made says `Suggesting…` in the present participle of 2.4 and nothing more.

---

## 8. Where this binds

* **`locales/en.json`** — every entry, at the moment it is added.
* **`packages/design-system/src/`** — components carry no sentences at all; a component that needs
  one takes it as a prop from a code the caller resolved.
* **`apps/webapp`** — the renderer of those codes, and the client copy that has no backend code
  behind it: button labels, empty states, and the frame's own words.
* **Not the API's own field names, the CLI's usage text, or this repository's documentation** —
  each has its own conventions.
