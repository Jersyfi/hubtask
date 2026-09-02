# Voice and Tone — the writing rules

Foundations v0.1 · The counterpart to [`design-system.md`](./design-system.md) for words

---

## 0. What this is

The rules a component author — or an assistant writing a pull request — applies without asking
anybody. Not an essay on style: every section below is stated so that a reviewer can point at a
number and say a label breaks it.

It covers the three places wave 1 puts words: **buttons**, **errors** and **empty states**. It
does not cover documentation, the website, or commit messages.

**Every string here is a catalogue entry, not a sentence in a component.**
[ADR-0011](../adr/ADR-0011-i18n-message-codes.md) and
[`i18n-l10n.md`](../architecture/i18n-l10n.md) §1: the server emits a code plus parameters and
never a finished sentence, and the client renders it from `locales/en.json`. So a writing rule is a
rule about a catalogue entry, and **a component that hard-codes a sentence has broken two rules
rather than one** — this page, and the one that makes the product translatable at all.

That has a consequence worth stating on its own, because it is the rule most easily broken by
accident: **English is the source language, not the only one.** A sentence that only works because
English puts the verb where it does, or that reads as a joke in English and as nonsense elsewhere,
is a sentence the translator cannot save.

---

## 1. Case, punctuation, person

**1.1 Sentence case. Everywhere.** Buttons, headings, labels, menu items, dialog titles, column
headers, tab labels. `Create task`, never `Create Task`. Title case is a second convention nobody
can apply consistently, it fights German capitalisation, and it makes a proper noun invisible.
Capitalise only the first word and what would be capitalised mid-sentence — a product name, a
person's name, `PostgreSQL`, `Europe/Berlin`.

**1.2 A full stop ends a sentence and nothing else.** Body copy, error messages and helper text
are sentences and take one. Button labels, headings, menu items, column headers, chips, tooltips of
one fragment, and email subject lines are not sentences and take none.

**1.3 A string that ends in an identifier ends without a full stop.** `Reference: {request_id}` —
the stop looks like part of the identifier the moment somebody copies it into a support ticket.
This is the one exception to 1.2 and the reason it is written down rather than discovered.

**1.4 Second person, singular, present tense.** "You do not have permission for this action."
Never "the user", never a passive that hides who does what.

**1.5 "We" appears only where Hubtask owns a failure.** `Something went wrong on our side` is the
sentence "we" exists for. Everywhere else the product is not a person and does not need a voice:
the sentence is about the reader's work, not about us.

**1.6 No exclamation marks doing the enthusiasm's work** (`design-system.md` §8). A celebration is
carried by the motion the design system defines (§7), not by punctuation. A sentence that needs an
exclamation mark to feel positive is not a positive sentence.

**1.7 No "please" as a reflex.** It does not make an instruction politer; it makes it longer, and
it reads as apology where none is owed. It earns its place in exactly two cases: **Hubtask is
refusing something the reader is entitled to** ("Too many requests. Please try again in a moment.")
and **Hubtask is asking for work it caused** ("Please try again in a moment" after a dependency
failed). Asking somebody to sign in is neither.

---

## 2. Buttons

**2.1 The label is the verb of what happens.** `Create task`, `Archive`, `Send invitation`. Not
`OK`, not `Submit`, not `Yes`. A person reading only the button must be able to say what it will
do — which is also what makes a dialog readable when the question above it is skimmed.

**2.2 The verb and its object, where the object is not obvious from context.** `Delete` inside a
task's own menu; `Delete work package` in a confirmation that could be about any of five levels.

**2.3 The wording stays consistent through the whole flow.** What reads `Publish` reports
`Published` afterwards and appears as `Published` in the activity stream. Three words for one
action is three actions to a reader.

**2.4 While it is working, the label becomes the present participle of its own verb.** `Create
task` → `Creating…`, `Send invitation` → `Sending…`. Not `Please wait`, not a label that vanishes:
the button keeps its width and its place, because rule 6 forbids animating layout and a label that
changes length moves everything beside it.

**2.5 The cancelling button says what it does, not what it is.** `Cancel` is right when nothing has
happened yet. `Discard changes` is right when something would be lost — and then the destructive
button, not the safe one, carries the specific verb.

**2.6 A destructive button names what is destroyed.** `Delete permanently` where deletion is not
recoverable; the object where several are in reach. A person who is about to lose something reads
the button and nothing else.

---

## 3. Errors

**3.1 An error names the fix, not only the fault.** This is the rule with the most teeth on this
page, and the one the catalogue already keeps best:

> `bulk.no_operations` — "Say what the bulk should do — a bulk with no operations changes nothing."
> `crypto.no_encryption_key` — "This installation has no encryption key configured, so it cannot
> store that securely. Set HUBTASK_ENCRYPTION_KEYS."

Fault, consequence, fix — in that order, and the fix is a thing the reader can actually do.

**3.2 Where there is no fix, say so and stop.** `errors.not_found` — "This entry does not exist."
is finished. Inventing an instruction for a situation that has none is worse than the missing one.

**3.3 Never blame the reader, and never blame them by grammar either.** "You entered an invalid
date" and "The date could not be read" describe the same event; the second is the one that does not
make somebody feel stupid for a typo. Reserve "you" in an error for permissions and for what the
reader may do about it.

**3.4 The reader's vocabulary, not the system's.** `request`, `payload`, `entity`, `cursor`,
`null`, `parameter` and `serialisation` are how the code thinks. A message that reaches a person in
the interface uses the nouns the interface uses: entry, workspace, collection, task.

That rule has a boundary, and it is worth naming: **some codes are read by a developer through a
problem document, not by a person through the UI**. A code that only ever appears in an API answer
may use API vocabulary. A code that can reach both — and the generic `errors.*` fallbacks can —
must use the reader's.

**3.5 An error says what is still true.** "It stays unassigned", "Nothing is being delivered to it
until somebody switches it back on". The state after the failure is the thing a person needs in
order to decide whether to act now.

**3.6 A recoverable failure says when to try again**, and says it in the message rather than only
in a header: "Please try again in {retry_after_seconds} seconds."

---

## 4. Empty states

An empty list has three causes, they mean different things, and one sentence cannot serve all
three. Distinguishing them is what makes an empty state useful rather than decorative.

**4.1 Empty because nothing has been made yet.** Say what this place is for and offer the one
action that fills it. This is the only empty state that carries a call to action.

> No tasks in this collection yet. — `Create task`

**4.2 Empty because a filter or a search excluded everything.** Say that the filter did it, and
offer to widen it. Never the same copy as 4.1: offering to create something when eleven things
exist and are hidden is an answer to a question nobody asked.

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

`design-system.md` rule 4: everything grows by 40 %. German, Finnish and Russian break any layout
measured against English, and this page's strings are the strings that break it.

**5.1 A button label is one or two words wherever the language allows.** `Send invitation` is
already `Einladung versenden`.

**5.2 An error is one sentence, or two short ones.** Fault and fix. A third sentence is
documentation, and documentation belongs behind a link.

**5.3 Nothing is written to a measured width.** A string tuned so that it fits on one line in
English is a string that wraps badly in every other language and reads as a mistake.

---

## 6. Ten codes, checked

The rules above are worth what they catch, so here they are applied to ten entries of
`locales/en.json` as it stands on 2026-09-02. Fixing what disagrees is optional; hiding it is not.

| Code | Rules | Verdict |
|---|---|---|
| `bulk.no_operations` | 3.1, 3.4 | **Model entry.** Fix first, consequence second, and the vocabulary is the reader's. |
| `crypto.no_encryption_key` | 3.1 | **Model entry.** Fault, consequence, and a fix specific enough to act on. |
| `backup.no_parent_archive` | 3.1, 3.5 | **Model entry.** Says why, says what is still true, ends with the action. |
| `automation.action_not_available_yet` | 3.1, 3.3 | **Passes.** A capability that does not exist yet, said without making it sound like the reader's error. |
| `errors.not_found` | 3.2 | **Passes.** There is no fix; it does not invent one. |
| `errors.rate_limited` | 1.7, 3.6 | **Passes.** One of the two places "please" earns its place — Hubtask is refusing something the reader may do. |
| `errors.internal` | 1.3, 1.5 | **Passes,** and it is why 1.3 and 1.5 are written down: "our side" is the one place "we" belongs, and the reference ends without a full stop on purpose. |
| `errors.unauthenticated` | 1.7 | **Disagrees.** "Please sign in." — signing in is neither a refusal nor our fault. `Sign in.` |
| `errors.validation_failed` | 3.1, 3.4 | **Disagrees.** "The request contains invalid values." names neither the fix nor which value, in the vocabulary of the API. The field-level codes underneath it do both, so the finding is about this fallback reaching a person at all. |
| `errors.conflict` | 3.1, 3.4 | **Disagrees.** "This action conflicts with the current state." — "the current state" is the system describing itself. Compare `errors.version_conflict`, which says the same class of thing as "Someone else changed this entry in the meantime." and is immediately actionable. |

Two more worth naming, since the audit found them:

* `webhooks.target_rate_limited` — "The target asked us to slow down." Breaks 1.5: no failure of
  ours is being owned, so there is no "us" to speak of.
* `items.auto_assign_no_candidate` — "No candidate of the assignment policy can receive this entry
  at the moment." Breaks 3.4; "candidate of the assignment policy" is the model's vocabulary.

The catalogue holds no exclamation mark anywhere (1.6) and no title case in a sentence (1.1), which
is a better starting position than most.

---

## 7. Where this binds

* **`locales/en.json`** — every entry, at the moment it is added.
* **`packages/design-system/src/`** — components carry no sentences at all; a component that needs
  one takes it as a prop from a code the caller resolved.
* **`apps/webapp`** — the renderer of those codes (`F1-07`), and the client copy that has no
  backend code behind it: button labels, empty states, and the frame's own words.
* **Not the API's own field names, not the CLI's usage text, and not this repository's
  documentation** — each has its own conventions, and this page does not overrule them.
