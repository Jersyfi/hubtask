# Milestone CD — Custom domains

The goal: **every workspace of a multi-tenant installation has an address of its own that can
change** — a subdomain, a host the operator assigns, or a domain its owner sets up without the
operator — **and a person moves between their workspaces from the list their device keeps**. A
change of address is the operator's act always and the owner's where the installation allows it;
the old address is free at once, everybody is told what it costs, and nobody is locked out by it.
Single mode, and every installation that switches nothing on, sees no new control beyond the
operator's change of address and the device's list. Milestone SI prepared `tenant_host` and left
it unread; CD makes it the resolver.

The rules are decided in [multi-tenancy.md](../architecture/multi-tenancy.md) §3.1
([decision #1568](https://github.com/Jersyfi/hubtask/issues/1568)) and drafted in multi-tenancy.md
§3.1, [identity.md](../architecture/identity.md) §10.1, [security.md](../architecture/security.md)
§4 (T-28) and [deployment.md](../architecture/deployment.md) §6.1; the mechanism's reasoning is
[ADR-0095](../adr/ADR-0095-a-workspaces-address-resolves-through-its-hosts.md). The market research
behind the decision is archived in
[workspace-addressing-research-2026-10.md](../archive/backlog/workspace-addressing-research-2026-10.md).
Every task names its use cases; a task is done when the named checks hold, with the evidence in the
pull request.

**Delivers:** UC-INS-07 (2), UC-INS-15 (1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11), UC-INS-19 (1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11), UC-INS-20 (1, 2, 3, 4, 5), UC-ID-25 (1, 2, 3, 4, 5, 6, 8)
**Released:** 2026-10-28

## Decisions

Each measured against the deployments `D1`–`D7`:

1. **The owner's answers D-1 to D-8** (decision #1568, 2026-10-16) govern: hosts, never a path
   prefix, and operator-assigned hosts where there is no wildcard (D-1); the operator changes an
   address always, the owner where the installation allows it, and the old address is free at once
   with no redirect (D-2); no *Find your workspaces* (D-3) and no routing by an address's domain
   (D-5); the device's list in the web app and the shells (D-4); passkeys end on a change, and
   nobody is locked out by it (D-6); renaming comes with own domains, here (D-7); an owner sets up
   an own domain without the operator, who sees and may block every one, behind an installation
   switch, and the subdomain then stays and redirects (D-8).
2. **UC-INS-15 is reworded by decision #1568 (D-8)**: the owner sets up the domain, not an
   administrator by way of the operator; the certificate comes on demand; links use the main
   address and old ones survive through the subdomain's redirect, where check 4 said the
   provider's address; the operator's block and the way back are checks of their own; deployments
   `D5`–`D7`, because the switch is the installation's, not a kind of provider.
3. **`tenant_host` is the resolver, and every address Hubtask writes comes from a workspace's main
   row** (worker, at the cut; ADR-0095 §1, §2). Three mechanisms for a subdomain, an assigned host
   and an own domain would disagree about which host is the workspace's; one table already holds
   all three shapes.
4. **The switches start closed** (worker, at the cut): owners may change the address — off; owners
   may set up own domains — off; the cooldown — 30 days. Each is an installation setting with the
   instance file, `hubctl admin settings` and the installation's settings as its doors (P-07,
   P-08). An installation that switches nothing on behaves as decided for the operator alone.
5. **The certificate is the ingress's, on demand** (worker, at the cut; ADR-0095 §5): the
   installation answers on the operations port whether a host may have one. Hubtask holds no
   private key and adds no ACME dependency; the operations port is not public, so the answer is no
   lookup for strangers. deployment.md shows one ingress that asks.
6. **A change of address deletes nothing** (worker, at the cut): passkeys made at the old host stay
   stored and count as no factor, as identity.md §23.3 already holds for a credential of another
   host; the person sees them as such and can remove them. Personal access tokens, service
   accounts and OAuth grants are bound to the workspace, not the host, and keep working once
   pointed at the new address.
7. **The device's list is signed by the installation** (worker, at the cut; ADR-0095 §7), and the
   installation's own address offers no field that takes a short name: D-4 decided a list, not a
   way to reach an unknown workspace, and a list filled from a plain link could send a person to a
   look-alike host.
8. **A workspace whose short name becomes reserved keeps it** (worker, at the cut): the list
   refuses new names; renaming an existing workspace against its will is an operator's change of
   address (CD-05), not a side effect of an update.
9. **One own domain per workspace** (worker, at the cut): it covers D6's "the customer's own
   domain"; more would need a rule for which one is main and gains nobody anything.
10. **UC-ID-25 check 7, the installed apps' list, is F7's** (worker, at the cut): there is no
    installed app before F7. CD builds the web app's list and the signed entry the shells reuse.
11. **Every screen is shown to the owner before it is built** (worker, at the cut, as the owner
    asked for PL): a task that changes a screen starts with a preview the worker presents, and the
    screen's code follows the owner's word. Each such task says so in its *Acceptance*.

---

## CD-01 — Reserved short names

*Depends on: nothing.* · finding #1560 · multi-tenancy.md §3.1

**Use cases:** UC-INS-07 (2), UC-INS-19 (2)

The product's list of reserved short names lives in the domain — the labels an installation or its
ingress uses for itself (`www`, `api`, `auth`, `admin`, `app`, `mail`, `media`, `static`,
`status`, `docs`, `help` and their kin), matched with plurals — and the operator adds their own as
an installation setting through the instance file, `hubctl admin settings` and the installation's
settings. Provisioning by every door refuses a reserved name at the field with
`admin.slug_reserved`, separate from `admin.slug_invalid` and `admin.slug_taken`; the same check
serves the change of address (CD-05). A workspace that already carries a newly reserved name keeps
it (Decision 8).

**Acceptance:** domain table tests for the list (exact, plural, case); application tests that
provisioning through REST, `hubctl` and the instance file refuses a reserved name with the code and
accepts it once the operator's list no longer names it; an e2e walk of *New workspace* showing the
refusal at the field; UC-INS-07 back to `built` with its *Today* gone.

---

## CD-02 — A request's host names its workspace

*Depends on: nothing.* · ADR-0095 §1, §2 · multi-tenancy.md §3, §3.1 · identity.md §10.1

**Use cases:** UC-INS-19 (10)

An expand migration adds a narrow `SECURITY DEFINER` lookup that answers one workspace or none for
a host, and a door through which a multi-mode server's start gives every workspace its main row
where it has none (the subdomain, `ACTIVE`, with a mark) - the migrator does not know the
installation's host; both are entered in multi-tenancy.md §2.1's doors table (the restore drill's
list holds tables without row level security, and `tenant_host` keeps its own). `SessionWriter.resolveTenant`, the auth middleware's mismatch check and every
anonymous door resolve through it; the header stays the second source, and a host that names
another workspace than the credential stays `403 access.tenant_mismatch`. `oidc:locate` answers the
workspace's main address from the row, and the web app forwards to exactly the host the answer
names (identity.md §10.1). The passkey's RP ID already reads the row. ADR-0095 moves to `accepted`.

**Acceptance:** a Testcontainers test of the lookup (one, none, the base host, a host of a deleted
workspace) and a cross-tenant negative test; integration tests that sign-in, reset and the provider
return resolve a workspace by a row whose host is not its subdomain; a web app unit test that the
forward goes only to the named host; the migration test on a database with workspaces and no rows.

---

## CD-03 — Every mail and link from the main address

*Depends on: CD-02.* · finding #1549 · multi-tenancy.md §3.1

**Use cases:** UC-INS-19 (9)

One helper gives a workspace's address from its main row, and every writer of a link uses it: the
reset, the address confirmation, the notice that a way in changed, the password opening, every
notification link and the emergency access, beside the invitation, which already does. The
installation's own mails (operator notices) keep the installation's host. The helper replaces
`WorkspaceBase` built from the short name.

**Acceptance:** a test per mail kind that its link carries the main row's host after the row
changed; an architecture test that no mail writer reads `BaseURL` for a workspace's link; #1549
closed by the pull request.

---

## CD-04 — Hosts the operator assigns

*Depends on: CD-01, CD-02.* · ADR-0095 §6 · deployment.md §6.1

**Use cases:** UC-INS-20 (1, 2, 3, 5)

`HUBTASK_WORKSPACE_HOSTS=assigned` makes provisioning ask for a host instead of a short name, by
every door with the same checks: a host another workspace uses, the installation's own host and an
invalid name are refused with codes; no DNS proof is asked. The short name stays the workspace's
identifier for `hubctl` and the API, generated from the host where none is given. The sign-in card,
the invitation and every mail show the assigned host (through CD-03), and the provider's return
lands there (through CD-02). In subdomain mode and single mode nothing of it is shown; an
installation with workspaces does not switch between the two.

**Acceptance:** application tests of the refusals through REST, `hubctl` and the instance file; an
integration test that a workspace on an assigned host signs in, is invited and returns from a
provider there; an e2e walk of *New workspace* in assigned mode; the preview shown to the owner
first (Decision 11).

---

## CD-05 — The operator changes a workspace's address

*Depends on: CD-01, CD-02, CD-03.* · multi-tenancy.md §3.1 · ADR-0095 §3

**Use cases:** UC-INS-19 (1, 2, 4, 5, 6, 7, 8, 11), UC-INS-20 (4)

The contract first: `POST /admin/tenants/{id}:change-address` with the new short name (or host, in
assigned mode), which makes true the workspace settings' description that already promises it;
`hubctl admin tenant change-address`; the installation's workspace screen. One use case replaces the
main row and drops the old one in one transaction, so the old host answers like an unknown one at
once and its name is free. It is confirmed by a password step-up, or a fresh provider sign-in where
passwords are off, never a passkey; it is refused with a code while it would leave the person
making it, or the workspace's last owner, without a lasting way in (multi-tenancy.md §3.1, P-16).
The dialog names the new address and what ends. Every member with an address is mailed (old and
new address, passkeys ended, sign in again); a member without one is told at the next sign-in. The
change is in the trail and in the instance journal. A workspace set for deletion refuses it.

**Acceptance:** domain table tests for the lasting-way-in rule (password, sign-in name, provider,
factor required or not, passkey only); application tests that a passkey step-up is refused, the
last owner's lockout is refused, the old host resolves to nothing and a second workspace takes the
old name at once, the mails go out once; a cross-tenant negative test of the new repository
method; a `hubctl` test; an e2e walk of the dialog naming the costs; the preview shown to the owner
first (Decision 11).

---

## CD-06 — The owner changes the address, where the installation allows it

*Depends on: CD-05.* · multi-tenancy.md §3.1

**Use cases:** UC-INS-19 (3, 4, 5, 6)

An installation setting lets owners change the address (off by default) with a cooldown (30 days),
through the instance file, `hubctl admin settings` and the installation's settings. Where it is on,
the workspace's owner finds *Change address* in the workspace's address settings, the same dialog
and the same step-up and lockout rules as CD-05; a change inside the cooldown is refused with a code
and the screen names the day. Where it is off, the control is absent and the API refuses
(`access.address_change_not_allowed`).

**Acceptance:** application tests for the switch (off refuses, on admits, the installation's lock),
the cooldown with the `Clock` port, the owner's role required; an e2e walk as owner with the switch
on and off; the preview shown to the owner first (Decision 11).

---

## CD-07 — An own domain, proven by DNS

*Depends on: CD-02.* · ADR-0095 §4, §5 · security.md T-28 · deployment.md §6.1

**Use cases:** UC-INS-15 (1, 2, 3, 4, 5, 10, 11)

An installation setting lets owners set up own domains (off by default). The owner types a domain
in the workspace's address settings and sees two records with copy buttons — the TXT record with
the row's mark and the record that points at the installation's target — and that mail is not
affected. A domain beneath the installation's host, another workspace's host or a blocked one is
refused at the field. A DNS port with a deadline (rule 7) behind which a job checks pending domains
and re-checks verified ones; a lost proof turns the row `BROKEN`. The operations port answers the
ingress's question whether a host may have a certificate; deployment.md shows an ingress that asks.
The installation's workspace screen lists every own domain with its state, and the operator blocks
one with a reason (in the journal), which stops it answering at once. Every step is in the
workspace's trail.

**Acceptance:** domain table tests of the domain rules; application tests of the job with a fake
resolver (pending to verified, verified to broken, a block); an integration test of the operations
port's answer for verified, pending, blocked and unknown hosts; a cross-tenant negative test; an e2e
walk of the records and the states; the preview shown to the owner first (Decision 11).

---

## CD-08 — The own domain as the main address, and back

*Depends on: CD-05, CD-07.* · multi-tenancy.md §3.1 · ADR-0095 §3

**Use cases:** UC-INS-15 (6, 7, 8, 9, 11)

*Make it the main address* runs CD-05's confirmation — the costs, the step-up, the lockout rule —
and makes the verified domain the main row while the subdomain stays as a row that redirects the
browser to it and still answers the API; every link from then on carries the own domain. The owner
switches back the same way. A domain that turns broken or is blocked hands the main address back to
the subdomain in the same transaction, and the workspace's administration and the installation's
overview show the warning.

**Acceptance:** application tests that making main and switching back each replace the main row
once, that a broken or blocked main domain falls back in the same transaction; an integration test
that the subdomain redirects a browser request and answers an API request; an e2e walk of making
the domain main and of the warning after a break; the preview shown to the owner first (Decision
11).

---

## CD-09 — The device's list of workspaces

*Depends on: CD-02.* · ADR-0095 §7 · multi-tenancy.md §3.1

**Use cases:** UC-ID-25 (1, 2, 3, 4, 5, 6, 8)

After a sign-in in multi mode, the workspace sends the browser once to the installation's own host
with an entry — host and display name — signed by the installation's key; the page there checks it
with the server (its own signature, the host still a main address) and keeps it in its own storage.
The account menu's *Switch workspace* and the installation's own address show the list; choosing an
entry opens that host, where it signs in on its own; *Forget this workspace on this device* removes
it. No address and no token is stored, and the installation's address offers no field that looks a
workspace up (Decision 7). Single mode shows nothing of it.

**Acceptance:** an application test that the check accepts only the installation's own signature
and only a current main address; a web app unit test that the stored entry holds host and name
only; e2e walks of adding, switching, forgetting, and of an entry whose address changed; a walk in
single mode showing nothing; the preview shown to the owner first (Decision 11).

---

## The matrix

| Door | What CD changes |
|---|---|
| Web app | The address settings (change, own domain), the installation's workspace screen (change, own domains, block), the device's list and *Switch workspace*. |
| REST | `:change-address`, the own-domain routes of the workspace and the operator's block; every anonymous door resolves by host. |
| `hubctl` | `admin tenant change-address`, the settings by `admin instance`, own domains listed and blocked. |
| MCP, automation | Nothing new; a client set up on the old host stops working after a change, as the dialog says. |
| Background worker | The DNS verification job; the mails of a change. |
| Installation level | The four settings, the reserved list, `HUBTASK_WORKSPACE_HOSTS`, the ingress's question on the operations port. |

| Account | A change of address |
|---|---|
| Person with a password | Mailed; signs in again; confirms a change by the password. |
| Provider-only person | Mailed; signs in again; confirms by a fresh provider sign-in where passwords are off. |
| Passkey-only person | Refused as the one making the change (no lasting way in); as a member, mailed, and back in by a password reset or a provider where the workspace offers one - the lockout rule guards the one changing and the last owner (D-6). |
| Managed account (no address) | Told at the next sign-in. |
| Service account, personal access token, OAuth grant | Keeps working once its client points at the new address. |
| Suspended workspace | The operator may change it; its owner cannot, being unable to sign in. |
| Workspace set for deletion | Refused. |

Deployments: `D1`–`D3` run single mode and see none of it; `D4` uses subdomains or assigned hosts
and the operator's change; `D5` may let owners change the address; `D6` may let owners bring their
own domain; `D7` sets everything from the instance file and `hubctl`.

## The cut checklist

1. **What it delivers** — UC-INS-19, UC-INS-20 and UC-ID-25 are new (decision #1568, in
   `specified`); UC-INS-15 is reworded by decision #1568 (Decision 2); UC-INS-07 check 2 was
   promised and never met, so its state moves back to `partial` at the cut. Walked through `D1`–`D7`
   above. UC-ID-25 check 7 is F7's (Decision 10).
2. **Principles** — P-01 (an old address is free and answers like an unknown one; no screen names
   an address's workspaces), P-02 (the step-up is the account's lasting way, never a passkey), P-03
   (nothing deleted by a change; the subdomain as the way back), P-04 (the dialog names the costs),
   P-05 (switches closed, controls absent), P-07 (installation settings with locks), P-08 (every
   door), P-10 (single mode untouched), P-11 (every member told), P-16 (nobody locked out).
3. **The matrix** — above.
4. **The premise** — checked against `main` at d8443afee: `tenant_host` exists with four states, a
   mark and one canonical row, is written only at provisioning in multi mode, and is read only for
   the workspace's `hosts` and the passkey's RP ID; `resolve_tenant` reads the short name;
   `oidc:locate` builds from the short name and the web app forwards only beneath its own host; no
   reserved list exists although UC-INS-07 is `built` (#1560); every mail but the invitation links
   to the installation's host (#1549); the workspace settings' description promises a rename at
   `/admin/tenants` that does not exist; no ingress question, no assigned host and no device list
   exist; the installation's settings are a closed, typed set.
5. **Decisions** — above; the owner's answers are #1568; nothing of CD waits on the owner.
6. **The rules** — multi-tenancy.md §3.1 (how it resolves), identity.md §10.1 (the forward) and
   §23.3 (the use cases a changed host names), security.md T-28, deployment.md §6.1
   (`HUBTASK_WORKSPACE_HOSTS`, the certificate) drafted; ADR-0095 proposed, accepted when CD-02
   merges.
7. **Findings** — #1560 is CD-01, #1549 is CD-03. #1616 (no door gives a workspace an owner) is not
   CD's; CD-05 counts the owners there are. #1505 is about restored test slugs.
8. **Tasks** — each names its checks, dependencies and acceptance; together they carry `Delivers`.
9. **The standing rules** — rule 1 and rule 2 (the address rules in the domain, authorised in the
   application layer; the lookup is a door, not an authorisation), rule 3 (`tenant_host` under row
   security, the lookup a narrow definer), rule 4 (the cooldown and the job from `Clock`), rule 6
   and rule 7 (DNS through a port with a deadline), rule 8 (every refusal a code), rule 10 (no host
   or domain in a metric label; a domain is configuration, not user content, but stays out of
   logs beyond the journal), rule 11 and rule 12 (the contract first, an expand-only backfill);
   known traps: a read-then-write without a lock (two changes at once, the last owner), writer
   copies in `cmd/server/main.go`, a redirect built from the request.
10. **A look before the build** — CD-04 to CD-09 change screens; each starts with a preview the
    owner sees (Decision 11).
