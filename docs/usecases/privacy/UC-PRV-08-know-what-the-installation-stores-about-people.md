---
id: UC-PRV-08
title: Know what the installation stores about people, and how each piece is deleted
context: privacy
actors: [PE-auditor, PE-operator, PE-owner, PE-selfhoster]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-03, P-09, P-11]
state: partial
tasks: [E-10, F4-20]
checked_by: [test/privacy/PG7_catalogue_test.go, test/privacy/PG2_deletion_test.go, test/privacy/PG1_classification_test.go]
---

# Know what the installation stores about people, and how each piece is deleted

## Goal

Whoever is responsible — a data protection officer, a provider, a person running Hubtask for their
family — can read, for every place Hubtask keeps something about a person, what it is, why it is
kept, for how long and how it goes; and no new place can be added to the product without that
answer being written down first.

## Story

A company's data protection officer prepares the record of processing activities (Art. 30 GDPR).
She opens the data catalogue that ships with the version they run: every table and location that
holds personal data, its classification, its purpose, its legal basis, its retention and its
deletion path. She copies what applies into her own record. A family that runs Hubtask at home reads
the same page to know what an erasure request would have to reach.

## How to check

1. `docs/privacy/data-catalog.md` has a row for every table that holds a column with personal
   content, with classification, purpose, retention and deletion path; the PG-7 gate fails on a
   table without one.
2. Every deletion path in the catalogue is one the product carries out — cascade with the parent,
   anonymisation, a retention job, hash only, or the trail's own retention — and the PG-2 gate
   proves the cascades.
3. Each classification is one of the documented classes, and the PG-1 gate holds the columns to
   them.
4. The catalogue says plainly that it describes the software, not an operator's own record, and
   which parts — backups, logs, the operator's infrastructure — are the operator's.
5. A pull request that adds a table with personal content cannot reach `main` without its row.

## Where it ends

* Hubtask does not produce the operator's record of processing for them; it gives them the part
  only the software can know.
* No legal assessment of the legal bases for a given operator.

## Today

* **Check 5 fails.** The PG gates run in the nightly suite, not on every pull request: milestone SI
  merged four tables without rows (`account_password_history`, `identity_provider`, `operator`,
  `instance_setting`) and only the nightly `matrix-arm64` job noticed — issue #246, whose fix adds
  the rows. Moving the PG gates into the pull request check changes a gate and is the owner's
  decision.
