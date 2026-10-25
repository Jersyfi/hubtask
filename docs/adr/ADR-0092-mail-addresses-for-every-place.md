# ADR-0092 — Mail addresses for every place, on the operator's receiving domain

**Status:** accepted · **Date:** 2026-10-25 · **Decided:** 2026-10-24 and 2026-10-25, by the owner
("Die Dritte soll auch bestehen bleiben, dass man keinen eigenen E-Mail Server braucht, aber es muss
eine Möglichkeit geben, … vollumfängliches Feature"; "ADRs und Decisions hierzu sind freigegeben")

**Rule lives in:** [automation.md](../architecture/automation.md) §5.1,
[multi-tenancy.md](../architecture/multi-tenancy.md) §2.1 (the door `resolve_mail_address`),
[UC-JUM-09](../usecases/jumble/UC-JUM-09-mail-straight-into-a-collection-or-an-entry.md),
[UC-JUM-10](../usecases/jumble/UC-JUM-10-answer-a-notification-by-mail.md)

## Context

Mail reaches Hubtask through one door: `POST /jumble/mail/{token}`, a token-protected URL per
workspace that the operator's mail route posts raw messages to ([ADR-0040](./ADR-0040-no-imap-intake.md)).
Everything lands in the jumble. The owner's walk of round 2 asked for more: mail must reach a
collection as a new entry and an entry as a comment with its files, the address must stand where a
person looks for it, and still no installation may need a mail server of its own. The concept
(round 2, §6) left one question to the worker: a feature of the system, or automations shipped as
defaults.

## Decision

1. **A feature of the system, not shipped automations.** Every place — the jumble, a collection, an
   entry — has its address the moment somebody asks for it, without a rule anybody writes.
   Automations keep what lies beyond: they react to *entry created, channel e-mail* like to any
   other creation.
2. **One receiving domain for the installation, no mail server.** The operator sets
   `HUBTASK_MAIL_INBOUND_DOMAIN` (for example `in.hubtask.app`) and `HUBTASK_MAIL_INBOUND_KEY`. The
   domain's MX points at an inbound service (Postmark, Mailgun, SES, Cloudflare Email Routing) that
   posts each raw message to `POST /mail/inbound`, authenticated by that key as a bearer token or as
   the password of the URL's basic credentials. ADR-0040 stands: no IMAP, no stored mailbox
   password, the parser stays transport-independent.
3. **An address is a key.** `<label>.<code>@<domain>`: the label is the place's name made
   readable and plays no part in routing; the code is 16 characters of lowercase base32
   (80 bits), drawn at random and never derived from anything. One table holds them
   (`mail_address`: target, code, sender policy, who made it, when it was revoked), behind row
   level security like every other. Because the inbound route has no tenant, the narrow door
   `resolve_mail_address(code)` answers one tenant or none — the way `resolve_tenant` does for
   sign-in. *A new address* revokes the old one in the same statement. The code is kept readable
   rather than hashed, because a member copies it again whenever they need it; what bounds it is
   the place's own read permission.
4. **Archived or trashed means closed.** A delivery to a collection or an entry that is archived or
   in the trash is answered not-found and stores nothing; restored, the same address works again.
   The address is not touched — the state of the place decides.
5. **Who may send.** Per address, `MEMBERS` (the default) or `ANYONE`. The sender is recognised by
   the address of an active person account of the workspace, and only where the inbound service did
   not report SPF or DMARC as failed. A recognised sender acts as themselves, through the same use
   cases a click goes through (`CreateWorkItem`, `AddComment`, `AttachMedia`), with their own rights
   checked there. Where the sender is unknown, not allowed, or the message is marked as spam, the
   mail lands in the jumble *addressed to* the place, with *Sort in → place* offered. Under
   `ANYONE`, an unknown sender's mail is written directly as the account that opened the address to
   everyone, its text headed *From: Name &lt;address&gt;* — that person took the responsibility, and
   their rights bound it.
6. **What a mail becomes.** At a collection: an entry — subject → title, text → description with
   quoted replies cut, files → attachments. At an entry: a comment with the text, the files on the
   entry. At the jumble: a jumble entry, as today.
7. **Protection.** The parser's bounds (size, parts, files) stay. A message whose `Auto-Submitted`
   is anything but `no`, or that carries Hubtask's own notification header, is accepted and
   discarded, so a loop ends at the first turn. Each address takes at most 60 deliveries an hour;
   beyond that the route answers 429 and the inbound service retries later.
8. **Replies.** A notification about an entry carries that entry's address as `Reply-To` where a
   receiving domain is set. An answer from the recipient becomes a comment, by decision 5.
9. **Without a receiving domain** the places show no address field but a line for administrators
   that names the setting. The jumble's webhook door keeps working either way.

## Options

1. **A system feature with code addresses on one domain (chosen).**
2. **Default automations per collection.** Every collection would carry a rule somebody can break,
   disable or delete, archive and trash would have to reach into rules, and the address would live
   in the automation screen instead of where a person looks for it.
3. **Plus addressing on the operator's own mailbox** (`team+k7f2q@example.com`). Needs a mailbox
   and a poller — ADR-0040's no.
4. **A subdomain per workspace** (`k7f2q@walk.in.hubtask.app`). Needs wildcard MX and wildcard
   inbound routes that not every inbound service offers; the door does the same with one domain.

## Why

**The address belongs to the place.** A person forwards an invoice to *Büro* and expects it in
*Büro*, with the same rights and the same trash as everything else there. Rules model what a
workspace *decides*; an address is what a place *is*.

**The key is the address, the person is the author.** A `From` header authenticates nothing. What
lets content in is the unguessable code; what the sender changes is only whose name stands on it and
whose rights bound it — and the inbound service's SPF and DMARC verdict takes even that away from a
forged sender. Somebody who knows a members-only address and forges a member's `From` still gets
only what that member could have done by hand, and an SPF failure sends it to the jumble.

**The jumble is the safety net.** Everything that is not clearly allowed lands where a person
decides about it, addressed to the place it was meant for. Nothing is lost and nothing walks in.

## Consequences

**Positive:** one route and one secret for the whole installation; addresses that work with every
inbound service that can post a raw message; no new dependency; automations see mail arrivals like
any other creation.

**Negative:** the operator configures a domain and an inbound service — a one-time task, documented
in `deploy/docker/.env.example`. Without them the places show a hint instead of an address.

**Reconsider when** an inbound service that matters cannot post raw messages, or when per-place
sender lists beyond *members* and *anyone* are asked for.
