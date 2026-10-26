# ADR-0094 — One mail intake, routed by handles and ticket keys

**Status:** accepted · **Date:** 2026-10-25 · **Decided:** 2026-10-25, by the owner ("Wir brauchen
eine Performante, klevere und Benutzerfreundliche Lösung …"; "bei Betreibern oder eigene Domain soll
es auch funktionieren, dass man in den Betreff die Eingabe tätigen kann … dass bereits auch JMAP
unterstützt wird. SMTP Empfang wird nicht benötigt … wofür ich dir hiermit alles freigebe") ·
**Supersedes:** [ADR-0040](./ADR-0040-no-imap-intake.md), [ADR-0092](./ADR-0092-mail-addresses-for-every-place.md)

**Rule lives in:** [automation.md](../architecture/automation.md) §5,
[UC-JUM-12](../usecases/jumble/UC-JUM-12-receive-mail-into-a-place.md),
[UC-JUM-10](../usecases/jumble/UC-JUM-10-answer-a-notification-by-mail.md),
[UC-JUM-11](../usecases/jumble/UC-JUM-11-set-up-the-mail-intake.md)

## Context

ADR-0092 gave every place a random address on the operator's receiving domain. It was safe and
unusable: nobody remembers or dictates `buero.k7f2q…@`, and a family with one Gmail or GMX mailbox
had no receiving domain at all. The owner asked for one intake that works from a self-hoster's
single cheap mailbox to a large operator, that can be switched off and configured, that sorts by
readable names and ticket-like numbers, and that decides who may write — members, an allow list,
anyone, and a block list of addresses and domains. ADR-0040 had ruled out IMAP for reasons that
the design below answers.

## Decision

1. **One intake per workspace, any number of doors.** A connected mailbox (IMAP, or JMAP where
   the provider speaks it) and the operator's inbound service (`POST /mail/inbound`) deliver the
   same raw message to the same pipeline. Hubtask runs no SMTP receiver. A mailbox signs in with
   a password (an app password where the provider asks for one), or through the provider's own
   sign-in: OAuth 2.0 with PKCE, then IMAP SASL XOAUTH2 with an access token drawn from a sealed
   refresh token. Google and Microsoft are presets - one button where the operator configured an
   app (`HUBTASK_MAIL_OAUTH_{GOOGLE,MICROSOFT}_CLIENT_ID`/`_SECRET`), an app of the administrator's
   own otherwise - and any other provider connects by its HTTPS endpoints, scopes and IMAP server.
   No vendor is required, none is privileged beyond a preset.
2. **Readable names instead of secret codes.** Every collection has a *handle* (`einkauf`),
   derived from its name, unique in the workspace and kept on a rename; every entry has a number
   in its collection, and the two are its *ticket key* `EINKAUF-42`, shown at the entry.
3. **One routing order; the first rule that matches decides:**
   1. a ticket key — `[EINKAUF-42]` or `EINKAUF-42` in the subject, or `+einkauf-42` /
      `einkauf-42.` in the address — makes the mail a comment on that entry;
   2. a handle — `#einkauf` in the subject, `+einkauf` on a mailbox address, `einkauf.<workspace>@`
      on the operator's domain — makes it a new entry in that collection;
   3. a sender rule the administrator set (`@lieferant.de` → *Einkauf*);
   4. the workspace's default collection, if it has one;
   5. otherwise the jumble, with the reason attached.
   The subject works in every set-up, which is what makes a mailbox without plus addresses (GMX,
   web.de) and the operator's shared domain equally usable. The routing token is taken out of the
   entry's title.
4. **Who may write is a level, with lists.** Per workspace: *members* (the default — an active
   account of the workspace, which then writes as itself), *members and allowed* (plus addresses
   and `@domains` on an allow list), or *anyone*. A block list of addresses and domains applies
   before every level and discards. A sender who is not a member writes through the account of the
   administrator who opened the intake, with a `From:` line above the text. A sender whose SPF,
   DKIM or DMARC failed is unknown; a message the provider marked as spam goes to the jumble.
5. **Nothing unclear is lost.** Whatever no rule may file goes to the jumble with its reason
   ("no handle found", "sender not allowed") and the place it was meant for.
6. **Every message once.** A receipt per workspace and `Message-ID` makes a redelivery a no-op and
   is the intake's log: when, from whom, what it became, and why.
7. **The acts are the catalogue's.** CreateWorkItem, AddComment and AttachMedia, as the sender's
   or the opener's account, with their rights and audit trail — the same as in ADR-0092.
8. **On or off, at both levels.** The operator decides which doors exist (the inbound domain, the
   mailbox connection); the workspace administrator switches the intake on, connects a mailbox and
   sets the level, the lists, the sender rules and the default under *Administration → E-mail*.

## Consequences

- ADR-0040's objections are answered: a mailbox's password is sealed like every other secret and
  revocable; a mailbox that fails reports to the administrators and on the health page; polling
  hangs off the mailboxes, through the job queue's leases, not off a list of every workspace.
- ADR-0092's random codes and the per-place address table go; Reply-To on a notification becomes
  the ticket key on the intake's address, which needs no secret because the sender level applies.
- A readable address can be guessed, as every `support@` can; the sender level is the protection.
- An OAuth connection is started by an administrator and finished by the same one within ten
  minutes, once: the state is kept as its hash, the PKCE verifier and an own app's secret are
  sealed, the code is traded through the egress guard, and nothing is stored before one IMAP
  sign-in with the token worked. A provider that rotates the refresh token gets the new one sealed
  in place; one that withdraws the grant shows as `mail.oauth_revoked` on the mailbox.
- Operators allow the providers' token and IMAP hosts in the egress allowlist. A Google app left
  in *Testing* hands out refresh tokens that expire after seven days; a published one does not.
- Custom domains are a later step on the same pipeline; signed Message-IDs for replies are a
  hardening on top of rule 3.1.
