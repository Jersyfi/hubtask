---
id: UC-WRK-12
title: Record the facts a team tracks, with custom fields
context: work
actors: [PE-admin, PE-member, PE-person, PE-integrator, PE-agent]
deployments: [D1, D3, D4, D5, D6, D7]
serves: [P-05, P-08, P-11, P-12]
state: built
tasks: [C-07, F3-10, K-03]
checked_by: [test/integration/custom_field_test.go, test/integration/custom_field_acceptance_test.go, core/domain/model/work/CustomField_test.go, core/application/service/work/SetCustomField_test.go]
---

# Record the facts a team tracks, with custom fields

## Goal

A collection records what its work needs beyond a title and a date — a priority, an estimate, a
customer number, a link, a person to ask — as typed fields, so that the values can be filled in,
filtered on and trusted.

## Story

The office's "Orders" collection defines `priority` (pick one of low, normal, high), `amount`
(a number), `customer_url` (a link) and `reviewer` (a person). Each order shows those rows in its
details and the team fills them in; a wrong value is refused on the spot. Later the team filters
the list by `priority = high`.

## How to check

1. A person who may change the collection's shape defines a field with a key, one of the kinds
   text, number, date, pick one, pick several, yes/no, person and link, the options for the two
   pick kinds, whether it is required, and which entry types carry it.
2. A second field with the same key in the same collection is refused with `fields.key_taken`; a
   pick field without options with `fields.options_required`.
3. An entry of a type the field applies to shows one row per field and saves each value on its own;
   numbers are read and shown in the reader's own number format.
4. A value of the wrong kind, a pick outside the options, or a person who cannot see the entry is
   refused with a code naming the field.
5. A required field cannot be cleared once set (`fields.value_required`); making a field required
   does not make older entries invalid.
6. The key and the kind of a field cannot be changed after it is defined; deleting a field stops
   it being offered and keeps the values it held out of view rather than destroying them.
7. Field values can be filtered on in the collection's filter and in saved views, with the
   operators that fit the kind (a pick field offers its options).
8. A member without the right to change the collection's shape sees the fields on entries and
   fills them in, but is not offered to define, change or delete them.

## Where it ends

* No computed fields and no formulas.
* No sorting or grouping by a custom field — the server orders only by its own columns.
* No display name separate from the key; the key is what people read.
* Workspace-wide fields are defined through the API; the collection's dialog shows them read-only.
* A field that decides behaviour (a status that drives a workflow) is an automation rule's job,
  not a field's.
