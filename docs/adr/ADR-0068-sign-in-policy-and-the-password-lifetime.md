# ADR-0068 — The sign-in rule: three levels, a lock, and the password over its lifetime

**Status:** proposed · **Date:** 2026-09-26

## Context

The sign-in screen was walked on 2026-09-21 and the concept that came out of it was approved in
six passes. Most of what it asks for is a client change. Four of its findings are not, and each is
a fact about this repository rather than an opinion about screens:

1. **Nobody can change their password.** The contract has `POST /auth/invitations:redeem`, which
   sets the *first* one, and nothing else. There is no change, no reset, no "I forgot". An account
   whose password is compromised can be disabled by an administrator or left as it is.
2. **The policy is a constant.** `core/domain/model/identity/Session.go` declares
   `MinPasswordLength = 12` and `MaxPasswordLength = 1024`, and `CheckPassword` compares against
   them. `security.md` §5 promises a blocklist check "(offline, optional)" that no code reads, and
   an installation that has to satisfy a rule demanding character classes, an expiry or a password
   history cannot: there is nowhere to say so.
3. **Three ways to be refused, one way to find out.** A password is checked where it is set, which
   means after it is typed and sent. Everything the server knows that the client does not — a
   blocklist, a history, a breach corpus — is a round trip whose answer is "no", with no way to
   say which of the rules was the one.
4. **The contract answers more than any client reads.** `MfaChallenge.expires_at` is the moment a
   pending credential dies and no screen shows it. `SessionTokens.recovery_codes_remaining` has
   carried "zero is the number to act on" since H-02 and no client had ever read it.

Two constraints frame every answer. **Nothing may enumerate tenants or accounts** — the project's
own rule, and the reason a policy change cannot be a job that walks rows. And **NIST SP 800-63B-4
advises against most of what a strict policy asks for** — composition rules and periodic expiry —
while PCI DSS 4.0 §8.3.9 requires an expiry where a password is the only factor. Both audiences
are real, and a product that serves only one of them excludes the other.

## Decision

### 1. A rule with eighteen switches, in three groups

The policy is an object, not a number. Thirteen switches for the password, two for the second
factor and the methods, two for sessions, and one action that is an event rather than a setting:

| Group | Switches |
|---|---|
| Password | `min_length`, `min_lowercase`, `min_uppercase`, `min_digits`, `min_symbols`, `min_classes`, `max_repeat`, `common_passwords`, `blocklist_file`, `context_words`, `breach_check`, `max_age_days`, `history_count`, `min_age_hours` |
| Second factor and methods | `mfa_required_for` (`NONE`/`ADMINS`/`EVERYONE`), `methods` |
| Sessions | `session_max_days`, `session_idle_minutes` |
| The action | `rotation_from` — a moment, set by "require a new password from everyone" |

`require_admin_totp` on the workspace is not renamed or removed: it is what `mfa_required_for`
derives from and writes back to, so no client and no stored row has to move.

**The four classes are counted separately *and* as "n of four."** They cannot be expressed in each
other — "one digit and one symbol" is not "two of four" — and both are what real policies ask for.
Counting is by Unicode category, so `Ω` is an uppercase letter and an emoji is "other"; the switch
carries the sentence that a script without letter case can never satisfy the first two.

### 2. Three levels, and a lock

```
product minimum  →  the instance's default and lock  →  the workspace, where no lock lies
```

The product's minimum is a constant in the domain: eight characters, at most 1024, history at most
ten, a session at most thirty days. Nobody goes below it, the operator included.

The instance sets a *default* and a *lock* per switch ([ADR-0070](./ADR-0070-the-instance-layer.md)
holds where that lives). Open means a workspace may tighten it; closed means the value applies and
the workspace's control is switched off **with the reason and with who set it** — never hidden. A
setting that is simply absent is a support request.

A workspace only ever tightens. The server enforces that field by field and answers the refusal
against the field.

### 3. Enforcement is a step of the sign-in, never a job

`MfaChallenge.methods` gains **`PASSWORD_CHANGE`**. A password that is right but no longer meets
the rule — too short for a tightened policy, older than `max_age_days`, older than `rotation_from`
— is answered `202` with that method, and the pending credential can do one thing: set a new
password. Confirming it *is* the sign-in, exactly as `ENROLL` already works.

This is the whole of the enforcement. **No job walks accounts and no job walks tenants**: the rule
is applied where the plaintext password is already in hand, which is at the moment somebody uses
it. A workspace of ten and a workspace of ten thousand cost the same, and a person whose password
already satisfies the new rule never notices it changed.

Sessions are the same shape: `created_at < rotation_from`, `created_at + session_max_days < now`
and `last_seen_at + session_idle_minutes < now` are three comparisons in the check the revocation
list already performs on every request.

### 4. Argon2 parameters are re-applied where the password is, too

`security.md` §5 says the parameters are "reviewed yearly" and nothing acts on a review. The hash
string carries them (`$argon2id$v=19$m=65536,t=3,p=2$…`); at sign-in the plaintext is in hand, so a
hash whose parameters differ from the current ones is recomputed. The review becomes an effect
instead of a note.

### 5. `SetPassword` is one use case with four doors

Invitation token, reset token, pending credential, or a bearer with a step-up. One use case, one
policy check, one audit action, one place where the history is written. Four use cases that each
set a password would be four places for the rule to be applied slightly differently.

**The step-up is the proof of the old password**, so there is no "current password" field beside
one: asking for both is asking twice for one thing (WCAG 2.2 SC 3.3.7).

### 6. Forgetting a password: a link, and the second factor still stands

`POST /auth/password:forgot` answers `202` for an address that holds an account and for one that
does not — the same answer, because which addresses have accounts is what a probe is after (T-02).
Behind it, a job on the queue the invitation already uses; an unreachable mail server never fails
the request.

The token is an `auth_pending` row with `purpose = 'RESET'`: 32 bytes, hashed under its own purpose
label, thirty minutes, single use. No new table shape — the pending-credential discipline is
exactly right for it.

`POST /auth/password:reset` answers the pair **or**, where the account has a second factor, the
`202` that asks for it. Control of a mailbox is one proof; it does not replace the one the account
already demanded.

### 7. The rules are data, and the client renders them

`GET /auth/sign-in-rules` is public the way sign-in is, resolves the workspace from the host, and
answers four things: the methods, the providers, what a password must meet, and the operator's
legal links. It deliberately does **not** answer the expiry, the history depth, the timeouts or
the lists — nothing a guesser could use.

Why a public route at all, when F4 refused one that would have existed only to hide a button: a
*configurable* hint is otherwise wrong. A screen that says "at least twelve characters" where the
workspace demands fifteen is a screen that lies, and the password is refused after it was typed.

Each rule becomes a message code with parameters (`auth.password_rule.*`), and the same codes are
what `field_errors[]` carries when a password is refused. One fact, one sentence, whether the
client predicted it or the server sent it. The catalogue gains ICU plurals for the first time;
both renderers already implement them.

`POST /auth/password:check` answers what only the server knows — the lists, the breach corpus, the
history, "not the one you have now" — and **demands the same proof the setting demands**. Without
it the route is an oracle that tells anybody whether a word is on a blocklist or in somebody's
history. It is asked once the local rules hold, once the typing stops, and once per value.

### 8. What is deliberately not offered

A regular expression (an admin-supplied one is a ReDoS vector and its violation has no sentence
that names the fix, SC 3.3.3) · keyboard and alphabet sequences (layout-dependent; the common-
password list catches the real cases) · forbidding spaces (NIST requires the opposite) · password
hints and security questions · a strength meter as a *requirement* · a reset performed by an
administrator (they invite again, which exists) · magic links as an everyday method · SMS codes.

### 9. What this changes in `security.md` §5

The row reading "no forced rotation" becomes **"not by default, and not recommended"**. The
mechanism exists because PCI DSS asks for it; it ships off, the setting carries the sentence that
says so, and the one-time rotation is an event with a reason rather than a calendar.

## Options

1. **A policy object with three levels and a lock (chosen).**
2. **A number in the environment.** What the first draft of the concept proposed. It cannot serve
   B2B and B2C on one installation, it cannot express "this value is fixed", and changing it is a
   rollout.
3. **A policy per workspace with no instance level.** Every workspace can weaken what the operator
   needs held, and five thousand workspaces are configured by hand.
4. **Profiles ("relaxed", "strict").** A second opinion with no reasoning attached; the moment a
   house needs eleven of a profile's twelve rules it is back to switches, and now there are two
   models.

## Consequences

**Positive.** An installation can be NIST-shaped or PCI-shaped without a fork. A workspace sees
what it may change and what it may not, and by whom. A person setting a password learns what is
wrong while typing rather than after sending. Nothing enumerates anything. The three unread
contract fields are read.

**Negative.** Eighteen switches is a screen with eighteen switches, and an installation that turns
everything on shows twelve lines under a password field — measured at 375 px, ~260 px tall. The
`password:check` route costs Argon2 comparisons, which is why it is bounded by the quiet timer, the
per-value cache and its own rate-limit bucket. `history_count` costs a table of old hashes and *n*
comparisons per set, which is why it is capped at ten.

**Countermeasures.** Every switch ships at the value NIST advises and carries the sentence that
says what turning it on costs, including "adds a line to every password screen". The list orders
local rules before the server's and steps met rules back in `text.subtle`, so at twelve lines only
what is left stands out. A shared fixture under `api/` holds rules, passwords and the violations
they produce, read by the Go test and by the client's — the two predictions cannot drift.

**What must never become a switch.** Exporting one's own data, deleting it, asking what is held,
the language, the time zone, and accessibility. Those are obligations, not settings.
