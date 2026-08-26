<!--
SPDX-License-Identifier: BUSL-1.1
Copyright (c) 2026 Jérôme Bastian Winkel
-->

# RB-GDPR-33 — A personal data breach

**Not an alert runbook.** Nothing here fires from a metric: a breach is noticed by a person, by a
report, or by a security advisory, and this describes what the operator does then. The runbooks
under `deploy/observability/runbooks/` answer alerts; this one answers Art. 33 and 34, and it lives
beside the data catalogue because that is the document it reads from.

**The clock.** The 72 hours of Art. 33 start when the controller *becomes aware* — not when the
incident began, and not when it is understood. So the first hour is spent establishing what has to
be reported, not on a fix, and this runbook is written in that order.

**Who is the controller.** The operator of this installation. If the installation is run for
somebody else, the operator is a processor and Art. 33(2) applies instead: they notify their
controller without undue delay, and the controller notifies the authority.

---

## 1. The three questions a notification needs

Art. 33(3) asks for the nature of the breach, the categories and approximate number of data
subjects and records, the likely consequences, and the measures taken. Two of those are judgement.
The three this installation can answer from its own data are:

1. **Which workspaces are affected?**
2. **Which categories of personal data?**
3. **Which period?**

The audit trail, the access log and the data catalogue are what answer them, and they were designed
to ([data-protection.md](../architecture/data-protection.md) §8). If the answer to any of the three
is "we cannot tell", that is itself part of the notification — say so rather than estimating.

## 2. Establish the period, and freeze it

Take the earliest moment the access could have begun and the latest it could have ended. Every
query below is bounded by that window; without a bound the trail returns the workspace's whole
history and nobody can read it.

```sql
-- One workspace's entries in the window, newest first. `SET LOCAL app.tenant_id` is what the
-- application does per request; a psql session run by the operator sets it once.
SET LOCAL app.tenant_id = '00000000-0000-0000-0000-000000000000';
SELECT occurred_at, action, outcome, actor_type, actor_id, target_type, target_id, context
FROM audit_log
WHERE tenant_id = current_setting('app.tenant_id')::uuid
  AND occurred_at >= timestamptz '2026-08-01 00:00+00'
  AND occurred_at <  timestamptz '2026-08-08 00:00+00'
ORDER BY occurred_at DESC;
```

**Before anything else, verify the chain for that window.** A trail that was tampered with answers
none of the three questions, and finding that out afterwards is worse than finding it out now:

```http
POST /audit:verify
{ "from": "2026-08-01T00:00:00Z", "to": "2026-08-08T00:00:00Z" }
```

A break is reported with the sequence numbers it lies between. Record the result either way — "the
chain verified for the window" is part of the evidence, and `audit.chain_broken` is a critical
finding in its own right.

## 3. Which workspaces

An installation-wide incident — a leaked database credential, a stolen backup archive, a
compromised host — affects every workspace the artefact contained. A credential-shaped incident
affects the workspaces the credential could reach:

```sql
-- Which workspaces one account could act in. An account belongs to exactly one workspace, so this
-- is a single row unless the same person holds several accounts.
SELECT DISTINCT m.tenant_id, t.slug
FROM membership m JOIN tenant t ON t.id = m.tenant_id
WHERE m.account_id = '…';

-- Which workspaces one personal access token could reach, and when it was last used.
SELECT a.tenant_id, t.slug, k.last_used_at, k.revoked_at, k.expires_at
FROM access_token k JOIN account a ON a.id = k.account_id JOIN tenant t ON t.id = a.tenant_id
WHERE k.token_prefix = 'hbt_pat_…';
```

For a stolen backup archive, the archive's own manifest is the authority on what it contained
([backup-restore.md](../architecture/backup-restore.md) §4): it names the workspace, the schema
version and the entity counts, and it is signed.

## 4. Which categories of personal data

Do not answer this from memory. The categories come from the data catalogue, and the queries above
say which tables were reached; [data-catalog.md](./data-catalog.md) maps each table to its class,
its purpose and its legal basis. A notification names the classes — `PERSONAL_BASIC`,
`PERSONAL_CONTENT`, `PERSONAL_TECHNICAL`, `SPECIAL_CATEGORY_RISK`, `SECRET` — and the number of
subjects, not a list of names.

```sql
-- Approximate number of data subjects in one workspace: the people, not the rows.
SELECT count(*) FROM account WHERE tenant_id = '…' AND status <> 'ANONYMIZED';
```

`SPECIAL_CATEGORY_RISK` deserves its own sentence in the notification when the reached tables carry
free text: this product does not ask for special categories, and a person may still have written
one into a note. That is why the class exists.

## 5. What to do, in order

1. **Stop it.** Revoke the credential (`DELETE /access-tokens/{id}`), rotate what leaked
   (`HUBTASK_ENCRYPTION_KEYS` supports a new key without a data migration —
   [ADR-0015](../adr/ADR-0015-security-baseline.md)), disable the account, or take the
   installation off the network. Note the time; it goes in the notification.
2. **Preserve the evidence.** Take a backup *now*, before any remediation writes over anything, and
   keep it out of the normal generational retention: a restore of an older generation is not
   evidence of what the trail said today.
3. **Answer the three questions** with §2 to §4, and write the answers down as you get them.
4. **Decide on notification.** Art. 33: the supervisory authority within 72 hours unless the breach
   is unlikely to result in a risk. Art. 34: the affected people, without undue delay, when the risk
   is high — unless the data was encrypted in a way that makes it unintelligible, which is the point
   of §1 of [tom.md](./tom.md).
5. **Notify.** Late is better than not: Art. 33(1) provides for a notification after 72 hours with
   the reasons for the delay.
6. **Record it.** What happened, when it was noticed, what was decided and why. A decision *not* to
   notify has to be documented as carefully as a notification.

## 6. What not to do

* **Do not delete the trail, and do not "clean up" the affected rows.** The trail is exempt from
  erasure for this reason, and the grants refuse it anyway ([audit.md](../architecture/audit.md) §3).
* **Do not use a data subject erasure to make the incident go away.** An erasure is a person's
  right, not an incident tool, and it writes an entry saying who ran it.
* **Do not put names into the incident record.** The trail already holds what is needed, and an
  incident document copied around an organisation is a second breach in the making.
* **Do not wait for certainty.** The 72 hours are not a deadline for a complete account: Art. 33(4)
  allows the information to be provided in phases.

## 7. Related

* [data-protection.md](../architecture/data-protection.md) §8 — the obligation, and why the trail
  is designed to be analysable
* [audit.md](../architecture/audit.md) §3, §5 — the hash chain and how it is verified
* [data-catalog.md](./data-catalog.md) — which table holds which category
* [tom.md](./tom.md) — the measures a notification refers to
* [security.md](../architecture/security.md) §14 — the technical incident process this sits on top of
* [RB-A19](../../deploy/observability/runbooks/RB-A19-dsr-deadline.md) — the other deadline this
  installation watches
