# ADR-0075 — A step-up proves the account with whatever it holds

**Status:** accepted · **Date:** 2026-10-01 · **Accepted:** 2026-10-01

## Context

A step-up ([`security.md`](../architecture/security.md), "Privileged actions") is the fresh proof
a privileged action demands beyond the session. Since H-02 it knows two methods, `PASSWORD` and
`TOTP`, and since SC-09 it offers only what the account holds: `PASSWORD` where there is a password,
`TOTP` where a second factor is on.

That leaves accounts with no way to prove themselves at all. An account that signs in only through
a provider and has no second factor holds neither. It is exactly the account deployments `D4`, `D5`
and `D6` produce for their administrators, and
[UC-ID-08](../usecases/identity/UC-ID-08-sign-in-with-my-organisations-directory.md) says no second
factor is stacked on top of the organisation's directory. Such an administrator can not change a
sign-in rule, configure a provider or replace their recovery codes. Three more gaps share the cause:

* Turning off the second factor asks for the password in its own request body (`DisableTotp`), not
  through the step-up, so a provider-only account with a factor cannot turn it off
  ([UC-ID-05](../usecases/identity/UC-ID-05-change-my-password.md) check 5).
* Somebody who lost their authenticator but kept their recovery codes cannot use a code as the
  proof for replacing it.
* SC-09 bridged the first gap by asking such an account to set up a second factor first. That forces
  a Hubtask factor on top of a company directory, which UC-ID-08 rules out.

The owner agreed the shape on 2026-10-01 after it was put to him against deployments `D1`–`D7`.

## Decision

### 1. Four methods

| Method | Who holds it | How it proves |
|---|---|---|
| `PASSWORD` | an account with a password | the password, checked as at sign-in |
| `TOTP` | an account with a second factor on | a current code |
| `RECOVERY` | an account with a second factor on and at least one recovery code left | a recovery code, **consumed** by the step-up exactly as by a sign-in |
| `PROVIDER` | an account with a connected identity at a provider that is switched on for its workspace | a fresh sign-in at that provider |

`stepup.Methods` answers the methods the account holds, in this order, and the refusal's
`params.methods` carries them as it does today. The client builds its prompt from that list and from
nothing else.

### 2. `PROVIDER` is a fresh sign-in, verified twice

The step-up starts an authorisation request at the provider with `prompt=login` and `max_age=0`,
bound to the session that asked (state and nonce as at sign-in). On the way back the server accepts
the proof only when:

* the ID token's subject is the identity **already connected to this account** — a different
  identity at the same provider is refused, never connected; and
* `auth_time` is present and no older than the step-up's own window.

A provider that ignores `max_age` or omits `auth_time` does not prove anything fresh, and the server
says so with a sentence (`auth.step_up_provider_not_fresh`) instead of trusting the round trip
([P-02](../vision/principles.md#p-02-an-account-is-opened-only-by-its-own-strongest-proof)). The proof
answers the same step-up token the other methods answer; the privileged action does not learn how it
was proven.

### 3. Every privileged action takes the step-up, and only the step-up

`DisableTotp` stops being the one action with its own proof. It takes a step-up token like every
other privileged action, so a provider-only account with a factor can turn it off with a code, a
recovery code or its provider. The request body's `password` stays accepted, marked `deprecated`,
for one release, and is then removed; a client that sends it keeps working until then.

### 4. Nothing changes for an account that has a password

`D1`–`D3` see the dialog they see today. The new methods only appear where the account holds them.

## Consequences

* The step-up dialog shows exactly the account's ways: "Confirm with Contoso Entra ID" for a
  provider; after the return the page says the confirmation holds and the action can be taken.
* The trail records the method of every step-up, as today; `RECOVERY` additionally records that a
  code was consumed.
* The OIDC adapter reads `auth_time` from the ID token; nothing new is a dependency.
* "Set up a second factor first" in the profile (SC-09) is retired once `PROVIDER` is built.

## Options considered

**A. Require a Hubtask second factor of every provider-only administrator.** Rejected: it stacks a
second structure on top of the organisation's directory, which UC-ID-08 rules out, and it is the
bridge SC-09 shipped, not an end state.

**B. Let a provider-only account set a password afterwards.** Rejected: a second, weaker door into
an account its organisation decided should open only through the directory (P-02).

**C. Accept the session alone for provider-only accounts.** Rejected: a stolen session would then
be enough for every privileged action, which is what the step-up exists against.

**D. Four methods, each taken where the account holds it (chosen).**
