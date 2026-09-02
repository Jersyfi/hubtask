# ADR-0040 — No IMAP intake: the webhook door is the mail door

**Status:** accepted · **Date:** 2026-09-02 · **Closes:** open point AM-1 (`automation.md` §5)

## Context

G-11 built the mail intake transport-first: `POST /jumble/mail/{token}` takes a whole message,
the parser takes bytes and knows nothing about how they arrived, and the use case behind it takes
the parser's answer and a token. An operator points their MTA, their provider's inbound route, or
any mail-to-webhook bridge at that URL, and the message becomes a jumble entry.

[AM-1](../architecture/automation.md) asked what else may reach that parser, and named the obvious
candidate: IMAP polling, the transport that needs no bridge at all. It also named, deliberately,
the four things an answer has to settle — which client library, where the mailbox password lives,
who polls without anything enumerating tenants, and what happens to a mailbox nobody can reach any
more. The question came due in `0.6.0` and it was always a decision about the product rather than
about the code: the port cut in G-11 is what makes any second transport an adapter.

## Decision

**No IMAP client is taken in.** The webhook door stays the only way a message reaches the jumble,
and AM-1 is closed by decision rather than by code.

The four questions are answered by not asking them: no library, no stored mailbox password, no
per-tenant poll job, no unreachable-mailbox warning — none of it exists to be got wrong.

**What stays open is the door, not the question.** The parser remains transport-independent, which
is the whole of what a second transport would need. When one is built, the protocol to build is
**JMAP** ([RFC 8620](https://www.rfc-editor.org/rfc/rfc8620), [RFC 8621](https://www.rfc-editor.org/rfc/rfc8621)) rather than IMAP.

## Options

1. **No IMAP; the webhook door is the mail door (chosen).**
2. **IMAP with a client library.** The transport an operator with a mailbox and no MTA would reach
   for first.
3. **A hand-written IMAP client.** No dependency, at the price of writing a parser for a stateful,
   thirty-year-old protocol with a dialect per server — in the one place where the input is least
   trusted.
4. **JMAP now.** The right protocol, at the wrong time: it needs the same dependency decision, and
   no deployment has asked for it.

## Why

**The bridge is a smaller burden than it looks.** Every path an operator would take to IMAP already
ends at an HTTP POST somewhere: an MTA can pipe to `curl`, the inbound routes of the mail providers
people actually use forward the raw message, and the mail-to-webhook bridges are one container. An
operator who can configure an IMAP account against a self-hosted installation can configure one of
those, and the ones who cannot were not going to run their own IMAP polling either.

**An IMAP client reads hostile input on behalf of every tenant.** That is the sentence AM-1 wrote
itself, and it is the reason this is a supply chain decision rather than a library choice. The
webhook door reads hostile input too — but it reads it behind the request bound, the rate limits and
the load shedder, in a code path shared with every other route, and the bytes arrive already framed
by `net/http`.

**Polling means holding somebody's mailbox password.** A per-tenant stored credential, sealed under
E-02, rotatable, maskable, and one more thing that can leak — and one this installation cannot
revoke, because it is a password to a system somebody else runs. The webhook token is minted here
and revoked here.

**The failure mode is quieter and therefore worse.** AM-1 named it: a poll that fails for a week is
an inbox not arriving, and nobody notices until somebody asks where their mail went. It is
answerable — a health warning and a notification to the administrator — but the answer is more
machinery to catch a failure the webhook transport reports at the moment it happens, as a status
code the sender sees.

**And the protocol question is moving.** JMAP is getting common: JSON over HTTP, push instead of
poll, and a session model that fits what this system already speaks — where IMAP is a stateful
session protocol with a dialect per server. Building an IMAP poller in `0.6.0` would be committing
to the older of the two at the moment the newer one is arriving, and the code would be the kind
nobody deletes.

## Consequences

**Positive:** no dependency in the least trusted path; no stored mailbox passwords; one intake to
document, secure and test rather than two; the roadmap's dependency list loses a candidate.

**Negative:** an operator with a mailbox and no way to forward from it cannot use the jumble by
mail. That is the cost, and it is stated rather than hidden: the answer for them today is a bridge.

**Countermeasure, and it is already built:** the port cut G-11 made. A second transport is a
producer of bytes and a source of a tenant, and nothing between it and the entry changes — so this
decision costs a later JMAP adapter nothing beyond its own ADR.

**Reconsider when** a deployment that matters runs on a JMAP-capable provider, or an operator
reports the bridge as a real blocker rather than a preference. Either one reopens this as a JMAP
question, not as an IMAP one.
