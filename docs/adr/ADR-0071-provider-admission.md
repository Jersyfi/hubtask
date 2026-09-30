# ADR-0071 — Who a sign-in provider admits: the directory, not the text of an address

**Status:** accepted · **Date:** 2026-09-29 · **Accepted:** 2026-09-30

## Context

SI-10 made identity providers plural at two levels and gave them one admission axis,
`provisioning`, with a list of `allowed_email_domains` beside it. The milestone's review against the
sign-in concept corrected what that axis means — `DOMAINS` admits only inside the list, `ANY` admits
every address the provider says it verified — and left the mechanism it rests on untouched.

The mechanism does not hold. Three facts, each read from the provider's own documentation:

1. **Microsoft Entra ID does not issue `email_verified`.** It is not in the v2.0 ID token claim set.
   `infrastructure/oidc` reads that claim and, absent, answers `false`; the domain refuses an
   unverified address in every mode. **No Microsoft account can sign in to Hubtask today**, in any
   configuration — and the `MICROSOFT` preset says `AddressesVerified: true`, which is a statement
   about the issuer that the code never consults. Before SI-10's correction the same gap failed the
   other way: an unverified address was provisioned a fresh account on every sign-in.
2. **Google says in its own words that the email domain is the wrong check.** "The domain of the
   `email` claim is insufficient to ensure that the account is managed by a domain or organization
   — you must verify the `hd` claim explicitly." `allowed_email_domains` is exactly the check Google
   warns against: a personal Google account can hold a verified address at any domain.
3. **Microsoft publishes a *templated* issuer for its multi-directory endpoints.** The discovery
   document at `/common/v2.0` answers
   `"issuer": "https://login.microsoftonline.com/{tenantid}/v2.0"`, and documents the rule: replace
   `{tenantid}` with the token's `tid` claim and then compare exactly. Hubtask refuses `common` at
   configuration time instead, because ADR-0036 §2 compares `iss` to the configured string.

The third is what makes the first two urgent rather than academic. The owner's requirement is two
deployments that Hubtask cannot serve today:

* **A person with a personal Microsoft account** signs in to a Hubtask somebody offers them.
* **A company using that offering** lets its own people in through *its* Microsoft directory.

Both are the same provider. What separates them is which directory the person comes from — and
`tid` is the claim that says so. Refusing `common` refuses both.

## The thing this decision is actually about

An address is a **name**. A directory is a **fact the provider vouched for**. Hubtask has been
admitting on the name.

`someone@acme.example` arriving with `email_verified: true` means the provider confirmed that this
person receives mail at that address. It does **not** mean acme.example runs their account, and
both Google and Microsoft say so explicitly. A workspace that wrote `acme.example` into
`allowed_email_domains` believes it admitted its own staff; what it admitted is anybody at any
provider who can verify an address in that domain.

## Decision

### 1. An identity carries its directory, and admission is decided on it

`port.Identity` gains two fields beside the subject and the address:

* **`Directory`** — the provider's own identifier for the organisation the person belongs to.
  Microsoft: the `tid` claim. Google: the `hd` claim, absent for a personal account. A generic
  issuer: empty, because there is no claim that means this and inventing one would be a guess.
* **`AddressAuthoritative`** — whether the provider vouches for the address **and** for the domain
  it sits in. Google: `email_verified` **and** an `hd` that matches the address's domain (a personal
  account is verified but not domain-authoritative). Microsoft: the `xms_edov` optional claim, which
  is documented as exactly this. A generic issuer: `email_verified`, which is what OIDC core gives
  and all it gives.

`EmailVerified` stays and keeps its meaning: the provider confirmed delivery. It is no longer the
only question asked.

### 2. A provider names the directories it admits, beside the domains

`identity_provider` gains `allowed_directories text[]`. Microsoft: tenant GUIDs, with
`9188040d-6c67-4c5b-b112-36a304b66dad` meaning personal Microsoft accounts — the value Microsoft
documents for exactly that. Google: Workspace domains, as `hd` answers them.

The two lists are read together, and which one decides is the preset's, not the workspace's:

| Preset | Directory claim | `DOMAINS` admits when |
|---|---|---|
| `MICROSOFT` | `tid` | the directory is in `allowed_directories` |
| `GOOGLE` | `hd` | the directory is in `allowed_directories` |
| `GENERIC` | — | the address is verified **and** its domain is in `allowed_email_domains` |

**A preset that has a directory claim ignores `allowed_email_domains` for admission.** Not "prefers"
— ignores. Two lists that both admit are two doors, and the weaker one decides. The domains list
keeps one job: it is what a `GENERIC` issuer has, and it stays there because for a self-hosted
Keycloak the address domain is the only thing the token offers.

### 3. A templated issuer is substituted, then compared exactly

ADR-0036 §2's rule — "`iss` equals the configured issuer exactly" — is **kept, not weakened**. What
changes is what it is compared against when a provider publishes a template.

When discovery answers an `issuer` containing `{tenantid}`:

1. The token's `tid` is read **before verification**, from the unverified payload.
2. It is substituted into the template.
3. The verifier is given that string, and compares `iss` to it **exactly**.
4. The signature is checked against the key set the *template's* discovery published.

What that guarantees: the provider signed the token, and its `iss` and `tid` agree. Reading `tid`
first is not trust — a token claiming another directory gets another expected issuer, and the
signature still has to come from Microsoft's key set. This is Microsoft's own documented algorithm,
and the reason it is safe is that **the substitution decides nothing**: `allowed_directories` does.

A template with no directory claim configured, or a preset with no `Directory` mapping, is refused
at configuration: a templated issuer whose directories nobody bounded admits every organisation in
the world, and that is a decision, not a default.

### 4. `INVITED_ONLY` is the one mode a directory does not gate

It admits anybody the provider vouched for and creates nothing: the bound is the accounts that
already exist here. A directory list on top of it is allowed and narrows it further, which is what a
platform wants when it offers one provider to everybody and still refuses one directory.

### 5. What a provider promises is checked, and rechecked

Configuration already refuses an issuer that cannot be reached or disagrees with its own metadata.
It now also refuses one whose discovery document offers **no signing algorithm this installation
accepts** — an issuer every one of whose tokens would fail verification, which without this check is
found out by the first person who tries to sign in.

The recheck is the discovery cache: metadata is refetched hourly, so a provider that changes under
an installation meets the same refusal at the next configuration rather than a signature failure
nobody can read, and the key set behind it refetches on its own when a token arrives signed by a key
it does not know (ADR-0036). That is the whole of "keeps working in future" that this product can
honestly promise about somebody else's service.

**There is deliberately no installation-wide provider health probe.** It would have to enumerate
every workspace's providers, and nothing in this product may enumerate tenants
([`multi-tenancy.md`](../architecture/multi-tenancy.md)) — the boundary is a database policy rather
than a role, and a probe that went around it would be the one caller that does.

## Consequences

* **Microsoft sign-in works.** Both cases: the personal-account directory, and a company's own.
* **A workspace that listed email domains for Google or Microsoft has its list read differently**,
  and the change is a tightening — from "anybody who can verify an address there" to "anybody in
  that directory". Migration 0107 leaves both columns; nothing is rewritten, and a row with a domain
  list and no directory list under a preset that has a directory claim admits **nobody** until
  somebody names a directory. Failing closed is the only direction that does not hand out accounts,
  and the screen says so.
* **ADR-0036 is refined, not superseded.** Its four numbered requirements stand; §2's "exactly" now
  has a sentence about what the comparand is for a templated issuer, and §2 carries a note pointing
  here so that nobody reading it alone gets the unrefined rule.
* **`GENERIC` is unchanged**, and is still the preset with no verified addresses and therefore no
  `INVITED_ONLY` and no place at installation level.
* **Apple is not added here.** Its client secret is a signed JWT that expires within six months,
  which is a credential-rotation problem rather than an admission one; it belongs to its own task.

## Options considered

**A. Refuse `common` and require one provider row per directory (what is built).** Honest and safe,
and it cannot serve a platform that offers "sign in with Microsoft" to customers it does not know
yet. Kept as the *recommendation* for a single company, which is why nothing about naming an exact
issuer changes.

**B. Accept `common` and match `iss` against a regular expression.** Rejected. It accepts every
Azure directory in the world with no list that says which, and the only thing left standing between
a stranger and an account is `provisioning`. It also writes a pattern this product invented into the
place ADR-0036 put an exact comparison.

**C. Accept `common`, substitute the published template, and bound it with a directory list
(chosen).** The substitution is the provider's documented rule and is not the gate; the list is the
gate. It serves all three deployments — one company, a platform for many, personal accounts — with
one mechanism and one place to look at to see who may come in.

## What this decision does not settle

Whether the installation's own provider may use a templated issuer at all. It may: an installation
provider with a directory list is exactly the platform case. The rule that bounded it here — a provider
with no verified addresses is never an installation provider — is replaced by the addendum below:
such a provider may be offered, as `INVITED_ONLY` only.

## Addendum, 2026-09-30 — connecting an existing account asks for its own proof

The concept review of 2026-09-30 found a way into other people's accounts that admission does not
close, and the owner decided it (E2 in `docs/backlog/milestone-SC.md`).

**The finding.** Under `DOMAINS` and `ANY`, a first arrival whose address matches an existing
account was *linked* to it on the provider's word, and the session opened without the account's
second factor. A workspace administrator may configure providers, so an administrator could point a
self-hosted issuer they control at the owner's address and sign in as the owner. The test that
shows it is `TestAProviderCannotOpenAnAccountThatHoldsAPasswordAndAFactor`.

**The decision.** The safeguard moves from the provider kind to the act of connecting:

1. **An account that already holds a credential** — a password, an armed second factor, or an
   identity from another provider — is connected to an arriving identity only after the account's
   own proof: its password, then its second factor if it has one. The card's step machine asks for
   it as one more step, `LINK`; the pending credential carries which provider and which subject it
   will connect. An account with no password but another provider cannot give that proof and is
   refused with a sentence that names the way in it has.
2. **An account with no credential yet** — invited, never signed in — is connected on arrival, as
   before. That is what "invited" means.
3. **With that in place, "a preset whose addresses this installation cannot vouch for may not be
   `INVITED_ONLY`" is withdrawn.** It made `GENERIC` more permissive, not less: the modes left to it
   claim accounts on the same signal *and* create new ones. `GENERIC` may be `INVITED_ONLY`.
4. **An installation provider without a directory claim may be offered only as `INVITED_ONLY`.**
   That replaces §8's "never an installation provider": a self-hosted issuer offered to every
   workspace admits the people each workspace invited, and nobody else.
5. **Configuring, changing, offering and removing a provider asks for a step-up**, like every other
   change to how people sign in.

The ladder itself — `INVITED_ONLY`, `DOMAINS`, `ANY` as "who comes in" — was confirmed by
`TestTheAdmissionLadderIsWhoComesIn` (E1) and is unchanged.
