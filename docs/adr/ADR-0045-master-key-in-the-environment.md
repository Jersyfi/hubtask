# ADR-0045 — The master key stays in the environment

**Status:** accepted · **Date:** 2026-09-04 · **Closes:** open point S-2 (`security.md` §16)

## Context

E-02 built envelope encryption the way [security.md](../architecture/security.md) §3 describes it:
every sealed value gets a data key of its own, that data key is wrapped under the installation's
master key, and the identifier of the master key is stored beside the ciphertext. The master keys
come from `HUBTASK_ENCRYPTION_KEYS` as a ring — one current, every predecessor still readable —
and `infrastructure/crypto` is the only package in the system that names a cipher. Everything
inwards of it sees `core/port/crypto`.

[S-2](../architecture/security.md) asked where that master key should live once the installation is
operated for somebody rather than by its author: the environment it is in today, a cloud KMS, or
Vault. The port cut in E-02 is what makes the question an adapter question — the package comment
says so — so what is being decided here is a threat model and an operating cost, not a shape.

**The threat that matters is a database dump plus a filesystem read.** A dump alone is already
worthless: the ring is not in the database. The question is what the second half of that pair is
worth to an attacker, and what each option would take away from them.

## Decision

**The master key stays in the environment.** `HUBTASK_ENCRYPTION_KEYS` remains the only source of
master key material. No KMS client is taken in, no Vault is operated, and no second adapter behind
`port.Encryptor` is written.

**The decision has a trigger rather than a review date.** It is revisited when either of these
becomes true, and not before:

* the installation runs on hardware the project does not control **and** the operator of that
  hardware is not the operator of Hubtask — a hosting platform's staff being able to read the
  process environment is a different sentence from the author being able to;
* a customer's compliance review asks for custody separation, in writing, as a condition.

Neither is true of `0.6.0`'s production, and the second one is the one that will arrive first.

## Options

1. **The environment keyring (chosen).** The key is in the process environment and in whatever
   placed it there — a Kubernetes Secret, a systemd unit, a `.env` file. Costs nothing, defends the
   cold half of the threat completely, and defends the warm half not at all.
2. **A cloud KMS.** The wrapping key never leaves the provider; every seal and unseal becomes a
   network call, cached or not. It buys an access log and revocation, and it buys them against an
   attacker who has already lost — because a process compromised badly enough to read the
   environment is a process that can simply ask the KMS to unwrap. It costs a dependency, an
   availability coupling on a path that must work for the application to start, and a second
   provider in the data processing agreement.
3. **Vault, self-operated.** The same properties as a KMS, with the provider replaced by a second
   stateful system to run, upgrade and back up — and with Vault's own unseal key moving the problem
   down one level rather than solving it. For one operator this is the most expensive option and
   the one most likely to be the cause of an outage rather than the defence against one.
4. **No encryption at all for the affected values.** Named to be refused: the values are integration
   credentials, webhook secrets, OAuth client secrets, identity provider secrets, MFA secrets and
   backup target credentials. Every one of them is a key to something outside this system.

## What this decision does not buy

It is worth writing down plainly, because the alternatives are usually sold as if they did: **none
of the three options defends against a live compromise of the application process.** A process that
can read the environment can also hold a KMS session or a Vault token, and the ciphertext it wants
is a function call away in all three designs. What the options differ in is the *cold* case — a
disk snapshot, a stolen backup, a database dump handed to the wrong person — and against that case
the environment keyring already does the work, because the material is not in any of those places.

What a KMS would genuinely add is the *record*: who unwrapped what, and the ability to revoke a key
without reaching the machine holding it. That is an operational property, not a secrecy one, and it
is worth paying for when somebody other than the operator has to be held to account for it. Hence
the trigger above.

## How a rotation completes

A ring is only half of rotation. Adding a key is safe today — the new key becomes current, new
values seal under it, and every predecessor stays readable — but **nothing in the system rewraps a
value that was sealed under an older key**, because the envelope was designed so that the master
key protects one data key per row rather than the rows themselves. That property is what makes
adding a key free; it is also why retiring one has never been possible. An operator can add master
keys forever and remove none, and a ring that only grows is a rotation that never finishes.

So this ADR decides the second half:

* **Re-sealing is a job per tenant, not a sweep over the installation.** Nothing may enumerate
  tenants for its own purposes ([multi-tenancy.md](../architecture/multi-tenancy.md)); the work is
  enqueued by an operator action that is already allowed to see the tenant list in multi mode, and
  each tenant's job runs under that tenant's transaction like every other job.
* **A re-seal rewrites the wrapping, never the plaintext's meaning.** The data key is unwrapped
  under the key named in the row and wrapped again under the current one. The value, its purpose
  binding and its row are untouched, so a re-seal is invisible to everything above the port.
* **Retirement is gated on a count, not on a calendar.** The procedure ends by asking how many
  sealed values still name the retiring key. Removing a key from the ring while a row still names
  it turns that row into `crypto.unknown_key` — a refusal, not a silent loss, but a refusal in the
  middle of somebody's integration.
* **The drill is the proof.** A rotation that has only been designed is a hypothesis, the same way
  A-20 treats a backup nobody has restored. The procedure is exercised against a real stack and its
  evidence recorded before this ADR's implementation half counts as done.

## Consequences

* `HUBTASK_ENCRYPTION_KEYS` stays the documented, and only, way to give an installation its keys —
  including for self-hosting, where any other answer would have been a burden nobody asked for.
* The operator procedure for a rotation is documented in `security.md` and is the same for every
  installation, hosted or self-hosted.
* An operator who wants their keys in a KMS today cannot have it. That is the accepted cost, and
  the trigger above is when it stops being acceptable.
* The re-seal path is new code and new operator surface, and it exists because of this decision
  rather than despite it: choosing the cheapest place to keep the key does not excuse never being
  able to change it.
