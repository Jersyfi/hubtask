# Standing decisions

The owner's decisions that hold beyond the task they were taken in. **Append-only**: an entry is
never edited; a reversal is a new entry that names the one it replaces. Before a session classes a
question as the owner's ([`ready/README.md`](./ready/README.md) rule 2), it searches this file and
every record's § 8 — a question already answered is cited, not asked again.

Decisions that belong to one task stay in that task's record; decisions that shape the architecture
are ADRs; rules about what the product is belong in `docs/vision/`, which only the owner changes.
This file holds what falls between: how the owner wants the work done, and product rules he stated
that are not yet in a document. Collected on 2026-10-04 from the sessions' private notes, where they
had lived until then.

| Date | Decision | Source |
|---|---|---|
| 2026-08-16 | Each backlog task starts in a fresh session; the state lives in git and the pull request, not in the session. A batch may run in one session when he asks for it. | owner, A-04 |
| 2026-08-18 | The server is supported as a container only (Linux, amd64 and arm64); PostgreSQL 16 and 17. | B-15, `milestone-0.2.0.md` |
| 2026-08-20 | Ask about rules and product scope; implementation trade-offs are the session's to decide, in line with the principles, and to write down with the reason. | owner, B-12 |
| 2026-08-24 | CI runs the full pipeline on every push, drafts included. Not to be proposed again. | owner |
| 2026-09-02 | NATS client accepted; IMAP closed (JMAP named as the open door); 400 days; anonymise. | H-11…H-15 |
| 2026-09-24 | A duplicate restore names its copy `Name (restored <date> <six characters of the run>)`, a fixed rule. | #790, PR #1027 |
| 2026-09-27 | No page numbers anywhere in the product, not even client-side over a list held whole: design to the constraint, do not change the rule. | owner |
| 2026-09-30 | Every feature proposal walks the deployments D1–D7 — what a person and what an administrator or operator meets in each — with the rejected alternatives. | owner |
| 2026-10-01 | The website names everything the project does or has decided to do; decided-but-unbuilt things carry a *Planned* chip. | PR #1102 |
| 2026-10-01 | Nothing is merged without the owner's word, given per pull request or for a named set. | owner |
| 2026-10-02 | As much as possible is fixed in the running work instead of being filed as an issue; only the owner's list (`ready/README.md` rule 2) goes to him, each with a worked-out proposal. | owner |
| 2026-10-03 | **Nobody is locked out of the platform and their data.** A change may alter *how* a person signs in, never *whether* they can. | ADR-0077 §4 |
| 2026-10-03 | The second factor's audit actions are one family, `auth.mfa_*`; a rename keeps the old name readable as an alias and never rewrites a stored entry. | SC-29 |
| 2026-10-03 | Walks against a real identity provider's tenant are not part of a milestone's evidence — there are no such accounts. A provider is walked against a stub or a self-hosted provider, and the evidence says so. | owner, SC-15 |
| 2026-10-04 | No code before the task's readiness record says `ready`; the owner's questions come in one message per cut, with proposals. | this file's pull request |
