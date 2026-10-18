# ADR-0085 — Passkeys, verified in-house

**Status:** accepted · **Date:** 2026-10-15

**Rule lives in:** [identity.md](../architecture/identity.md) §23

## Context

Milestone SI shaped the contract for passkeys and left the rest to the passkey milestone
([milestone SI](../archive/backlog/milestone-SI.md), "Passkeys"), naming one question as a
supply-chain decision: whether `go-webauthn` or a minimal verifier of our own checks the WebAuthn
ceremonies. Two more questions were the owner's, because they change what a person sees: whether a
passkey opens an account alone, and whether an account may hold more than one authenticator
(UC-ID-03 promised one).

What the server has to verify is narrow. Registration: a client data JSON (type, challenge,
origin), an attestation object in CBOR (format, statement, authenticator data), and in the
authenticator data the RP ID hash, the flags, the counter and the credential's COSE public key.
Sign-in: the same client data, the authenticator data, and a signature over both. Everything else a
WebAuthn library offers — attestation formats (packed, TPM, Android, Apple), the FIDO metadata
service, certificate chains — answers the question *which maker built this key*, which Hubtask does
not ask.

## Decision

1. **A minimal in-house verifier, no new dependency** (the owner, #1520). The reasoning of
   [ADR-0053](./ADR-0053-totp-qr-code.md) holds: fixed by a standard (WebAuthn Level 2 §7.1 and
   §7.2), bounded inputs, no version churn. `attestation: none`, so the CBOR it reads is the subset
   the attestation object and a COSE key use (maps, byte and text strings, integers, no tags, no
   indefinite lengths, a depth and size bound); ES256, EdDSA and RS256 from `crypto/ecdsa`,
   `crypto/ed25519` and `crypto/rsa`. It is an outbound adapter behind a port, so the core imports
   none of it (rule 1). It is proven against the specification's published examples, against
   malformed and truncated inputs (a fuzz target), and with real authenticators: a platform passkey
   and a FIDO2 security key, and a U2F key for the ES256 path.
2. **A passkey with user verification opens the account alone** and satisfies the second-factor
   rule; a credential that proves presence only is the second factor after the password (the owner,
   #1521). The server reads the UV flag of each ceremony.
3. **An account holds up to ten passkeys or security keys** beside at most one authenticator app,
   and two use cases say so: UC-ID-21 and UC-ID-22 (the owner, #1522).
4. **No workspace switch that demands passkeys** and refuses codes, for now (the owner,
   2026-10-15).

## Consequences

* No lockfile entry, no transitive CBOR, JWT or TPM code to watch; the cost is a verifier that must
  be exactly right, which is why it carries a fuzz target and a walk with real keys, and why a
  reviewer attacks its readiness record before the code.
* Attestation can never be added without a new decision: a workspace that one day wants to admit
  only certain keys needs the metadata service, and with it most of what this ADR declined.
* Passkeys belong to a workspace's own host. A workspace that moves to its own domain loses them,
  which UC-INS-15 already tells the administrator.
* The shells cannot use them until F7 finds their way to the workspace's origin.
