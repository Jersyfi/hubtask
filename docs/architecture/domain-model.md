# Domain Model

Complements [arc42.md](./arc42.md) chapters 5 and 8.1. Binding for the implementation of
`core/domain` and `core/application`.

---

## 1. The guiding idea: generalisation

The requirements name four levels with different feature sets (task = everything, activity =
reduced). Four entities would mean four repositories, four permission checks, four API resources,
and four sets of automation actions — and all of it again when a fifth level appears.

**The decision:** one aggregate root `WorkItem` with an `ItemType` and a **capability profile**
that defines per type which capabilities are permitted. Containers are modelled analogously as
`Container` with a `ContainerType` (`HUB`, `COLLECTION`).

```mermaid
graph TD
  T[Tenant] --> C1[Container: HUB]
  C1 --> C2[Container: COLLECTION]
  C2 --> I1[WorkItem: TASK]
  I1 --> I2[WorkItem: WORK_PACKAGE]
  I2 --> I3[WorkItem: ACTIVITY]
  C2 --> B[Bucket]
  C2 --> L[Label]
  C2 --> V[SavedView]
  I1 --> CM[Comment]
  I1 --> A[ActivityEntry]
  I1 --> R[Reminder / RecurrenceRule]
  I1 --> AS[Assignment]
```

The consequence: one table, one repository, one set of use cases, one API resource (`/items`) —
extension through configuration rather than through code.

---

## 2. The capability matrix

`ItemCapabilityProfile` is a domain policy. System-defined profiles are defaults; tenants may
narrow them (never widen them beyond the system boundary).

| Capability | TASK | WORK_PACKAGE | ACTIVITY | Note |
|---|:--:|:--:|:--:|:--:|
| `COMPLETION` (done/open) | ✔ | ✔ | ✔ | Mandatory for every type |
| `DUE_DATE` | ✔ | ✔ | ✔ | |
| `REMINDER` | ✔ | ✔ | ✔ | Predefined plus custom |
| `ASSIGNMENT` | ✔ | ✔ | ✔ | Activity: exactly one assignee |
| `MEMBERS` (several) | ✔ | ✔ | ✘ | |
| `BUCKET` | ✔ | ✘ | ✘ | Buckets apply to items directly under the collection |
| `NOTES` | ✔ | ✔ | ✘ | |
| `LABELS` | ✔ | ✔ | ✘ | |
| `COMMENTS` | ✔ | ✔ | ✘ | |
| `COVER` | ✔ | ✘ | ✘ | A colour or an image |
| `ATTACHMENTS` | ✔ | ✔ | ✘ | |
| `HISTORY` | ✔ | ✔ | ✔ | Compact history for activities |
| `RECURRENCE` | ✔ | ✘ | ✘ | A series applies to the whole subtree |
| `CUSTOM_FIELDS` | ✔ | ✔ | ✘ | |
| `CHILDREN` | `WORK_PACKAGE` | `ACTIVITY` | — | The permitted child types |
| `MAX_DEPTH` | 3 | 2 | 1 | Relative to the collection |

**The rule:** setting a field whose capability is not active for the type produces
`ErrCapabilityNotSupported` (HTTP 422, code `capability_not_supported`) — not silent ignoring.

An extension example: a new type `MILESTONE` = a new profile entry plus an adjustment of the
permitted child types. No schema change, no API change.

---

## 3. Aggregates and entities

### 3.1 `Tenant` (context: Identity & Access)

| Field | Type | Rules |
|---|---|---|
| `id` | UUIDv7 | |
| `slug` | string(3..40) | Unique, `^[a-z0-9-]+$` |
| `displayName` | string(1..200) | |
| `status` | `ACTIVE` \| `SUSPENDED` \| `PENDING_DELETION` | State transitions only through use cases |
| `defaultLocale` | BCP-47 | e.g. `de-DE` |
| `defaultTimeZone` | IANA | e.g. `Europe/Berlin` |
| `settings` | JSONB | Retention, feature toggles, automation quotas |

In self-hosting mode exactly one tenant exists (`SINGLE`), created automatically.

### 3.2 `Account`, `Membership`, `Group`

`Account` = a person or a service account.

| Field | Type | Rules |
|---|---|---|
| `id`, `tenantId` | UUIDv7 | |
| `kind` | `USER` \| `SERVICE_ACCOUNT` | |
| `email` | string | Unique per tenant (for `USER`) |
| `externalSubject` | string? | The OIDC `sub`, for JIT provisioning |
| `locale`, `timeZone` | BCP-47 / IANA | Override the tenant default |
| `status` | `ACTIVE` \| `INVITED` \| `DISABLED` | |

`Membership(accountId, scopeType, scopeId, role)` with `scopeType ∈ {TENANT, HUB, COLLECTION, ITEM}`.
The effective permission is the highest role along the path (inheritance downwards).
`Group(id, tenantId, name, members[])` — the target object for assignment strategies and
permissions.

Roles and rights (an extract):

| Role | Read | Write items | Structure (buckets/labels) | Members | Automation | Delete (container) |
|---|:--:|:--:|:--:|:--:|:--:|:--:|
| `OWNER` | ✔ | ✔ | ✔ | ✔ | ✔ | ✔ |
| `ADMIN` | ✔ | ✔ | ✔ | ✔ | ✔ | ✘ |
| `MEMBER` | ✔ | ✔ | ✘ | ✘ | Own rules | ✘ |
| `CONTRIBUTOR` | ✔ | Assigned only | ✘ | ✘ | ✘ | ✘ |
| `VIEWER` | ✔ | ✘ | ✘ | ✘ | ✘ | ✘ |
| `GUEST` | Shared items only | Comment | ✘ | ✘ | ✘ | ✘ |

### 3.3 `Container` (hub / collection)

| Field | Type | Rules |
|---|---|---|
| `id`, `tenantId` | UUIDv7 | |
| `type` | `HUB` \| `COLLECTION` | |
| `parentId` | UUIDv7? | `HUB` → `null`; `COLLECTION` → a `HUB` |
| `name` | string(1..200) | Unique per parent level (case-insensitive, Unicode NFC normalised) |
| `description` | text? | |
| `icon`, `color` | string? | |
| `orderKey` | string | Ordering within the parent context |
| `policies` | JSONB | `completionPolicy`, `defaultBucketId`, `capabilityOverrides`, `autoAssign` |
| `archivedAt`, `deletedAt` | timestamptz? | Lifecycle |
| `version` | int | Optimistic locking |

Invariants:
* I-C1: a `HUB` has no container parent; a `COLLECTION` has exactly one `HUB` as its parent.
* I-C2: deleting a container moves the entire subtree to the trash (a cascading soft delete with a shared `trashBatchId`, so that restoring is atomic).
* I-C3: an archived container is read-only; children inherit `effectiveArchived`.

### 3.4 `WorkItem` (the aggregate root)

| Field | Type | Rules |
|---|---|---|
| `id`, `tenantId` | UUIDv7 | |
| `collectionId` | UUIDv7 | Denormalised for queries and RLS performance |
| `type` | `TASK` \| `WORK_PACKAGE` \| `ACTIVITY` | Extensible |
| `parentId` | UUIDv7? | `TASK` → `null` (the parent is the collection) |
| `path` | string | A materialised path (`/<taskId>/<wpId>/…`) for subtree queries |
| `depth` | int | Derived, ≤ the type's `MAX_DEPTH` |
| `title` | string(1..500) | Required, trimmed, non-empty |
| `notes` | text? | Markdown (not rendered server-side), capability-dependent |
| `completion` | `{isCompleted, completedAt, completedBy}` | |
| `bucketId` | UUIDv7? | `TASK` only; the bucket must belong to the collection |
| `orderKey` | string | Rank within the bucket or the parent item |
| `dueAt` | timestamptz? | Plus `dueDateOnly bool` (all day) and `dueTimeZone` |
| `startAt` | timestamptz? | For the timeline view |
| `labels` | Set\<labelId\> | The collection's labels |
| `members` | Set\<accountId\> | Capability `MEMBERS` |
| `assigneeId` | accountId? | Capability `ASSIGNMENT` (for an activity, exactly this field) |
| `cover` | `{kind: COLOR\|IMAGE, colorToken?, mediaId?}` | Capability `COVER` |
| `customFields` | JSONB | Validated against `CustomFieldDefinition` |
| `recurrenceRuleId` | UUIDv7? | Capability `RECURRENCE` |
| `originJumbleEntryId` | UUIDv7? | Provenance |
| `archivedAt`, `deletedAt`, `trashBatchId` | | Lifecycle |
| `createdBy`, `createdAt`, `updatedAt`, `version` | | Audit + locking |

Invariants:
* I-W1: the `type` must be permitted under the `parent` by the capability profile (`Hierarchy.Validate`).
* I-W2: no cycles; `path` and `depth` stay consistent — changes go exclusively through `Hierarchy.Move`, which updates the subtree within one transaction.
* I-W3: all references (bucket, label, assignee, member, media) live in the same tenant and — for buckets and labels — in the same collection.
* I-W4: a trashed or archived item is not editable except through `Restore`/`Unarchive`.
* I-W5: with `completionPolicy = ROLLUP` active, a parent item is automatically completed or reopened when the completion status of all its children changes (idempotent, event-driven).
* I-W6: moving an item to another collection is only permitted if every reference can be resolved there; unresolvable labels/buckets are removed and reported back in the result (not silently).
* I-W7: `title` and text fields are Unicode NFC normalised; length limits count code points, not bytes.

The lifecycle state machine:

```mermaid
stateDiagram-v2
  [*] --> Active
  Active --> Archived: Archive
  Archived --> Active: Unarchive
  Active --> Trashed: Delete
  Archived --> Trashed: Delete
  Trashed --> Active: Restore (< 30 days)
  Trashed --> [*]: Hard delete (retention job)
```

### 3.5 Surrounding entities

| Entity | Key fields | Rules |
|---|---|---|
| `Bucket` | `collectionId`, `name`, `orderKey`, `wipLimit?`, `isDoneBucket` | The "list" from the requirements; `isDoneBucket` can trigger completion |
| `Label` | `collectionId`, `name`, `colorToken`, `description?` | The colour is a token (not hex) → theming is possible in the frontend; hex optionally allowed |
| `Comment` | `itemId`, `authorId`, `body`, `editedAt?`, `parentCommentId?` | Only the author or an admin may change it; deletion is a soft delete ("removed") |
| `ActivityEntry` | `itemId`, `actor{type,id}`, `verb`, `changeSet` (JSONB), `occurredAt`, `causationId` | Append-only, the source of the history; `verb` is a code (i18n) |
| `MediaObject` | `tenantId`, `storageKey`, `mimeType`, `size`, `checksum`, `usage` | Presigned upload, reference counting, deletion on hard delete |
| `CustomFieldDefinition` | `scope(collection\|tenant)`, `key`, `kind` (`TEXT`,`NUMBER`,`DATE`,`SELECT`,`MULTI_SELECT`,`BOOL`,`USER`,`URL`), `options`, `required` | Enables extension without a migration |
| `Reminder` | `itemId`, `offsetSpec` (`REL:-PT1H` / `ABS:<ts>`), `channels[]`, `recipients[]`, `state` | Predefined (relative presets) and custom |
| `RecurrenceRule` | `rrule` (RFC 5545), `timeZone`, `mode` (`ON_SCHEDULE`\|`ON_COMPLETION`), `horizonDays`, `endSpec` | See arc42 §6.3 |
| `Template` | `scope`, `name`, `rootType`, `nodes[]` (a tree with fields, relative due dates `+3d`, assignment rules) | Instantiation produces an item tree |
| `SavedView` | `scope`, `name`, `layout` (`LIST_COLLAPSED`,`LIST_EXPANDED`,`KANBAN`,`TIMELINE`), `query`, `grouping`, `visibleFields`, `sharing` | `layout` is a hint; the server does not interpret it |
| `JumbleEntry` | `tenantId`, `channel` (`EMAIL`,`WEBHOOK`,`QUICK_CAPTURE`,`API`), `rawSubject`, `rawBody`, `attachments[]`, `sender`, `status` (`NEW`,`PROCESSED`,`DISMISSED`), `suggestion` (JSONB) | Conversion produces a `WorkItem` and sets `PROCESSED` |
| `AutoAssignPolicy` | `scope`, `strategy`, `candidates[]` (accounts/groups), `state` (for round robin) | See below |
| `AutomationRule` | See [automation.md](./automation.md) | |
| `WebhookSubscription` | `tenantId`, `targetUrl`, `eventTypes[]`, `secret`, `state`, `failureCount` | HMAC-SHA256 signature, auto-disable after sustained failure |

### 3.6 Automatic assignment

| Strategy | Behaviour | Determinism in tests |
|---|---|---|
| `FIXED` | Always the same person or group | Trivial |
| `RANDOM_MEMBER` | Random from the candidate list | Injectable through the `RandomSource` port |
| `RANDOM_GROUP_MEMBER` | A random group, then a random member | Likewise |
| `ROUND_ROBIN` | Rotating, with state persisted per policy | The state is explicitly visible |
| `LEAST_LOADED` | The lowest number of open items | A pure function over counts |

Extension happens through the `AssignmentStrategy` port; new strategies need no change to
`WorkItem`.

---

## 4. Domain events

The naming scheme: `de.hubtask.<context>.<entity>.<action>.v1`. Every event contains
`tenantId`, `actor{type,id}`, `occurredAt`, `correlationId`, `causationId`, `causationDepth`,
and a business payload. Events are a public contract (webhooks, automation, n8n).

| Event | Payload (core) | Consumers |
|---|---|---|
| `container.created` / `.renamed` / `.policies_updated` / `.moved` / `.archived` / `.unarchived` / `.deleted` / `.restored` | A container snapshot, `effectiveArchived` included; `.renamed` and `.policies_updated` add a `changeSet`, `.moved` the hub it came from | Automation, webhooks, search |
| `item.created` | An item snapshot, `parentRef` | Automation, search, SSE |
| `item.updated` | `changeSet` (old/new per field) | Automation (field change triggers), history |
| `item.completed` / `item.reopened` | `completedBy`, `completedAt` | Automation, roll-up, `ON_COMPLETION` recurrence |
| `item.moved` | `fromParent`, `toParent`, `fromBucket`, `toBucket`, `orderKey` | Kanban automation |
| `item.assigned` / `item.unassigned` | `assigneeId`, `strategy?` | Notification |
| `item.member_added` / `.member_removed` | `accountId` | Notification |
| `item.label_added` / `.label_removed` | `labelId` | Automation |
| `item.due_changed` | `oldDueAt`, `newDueAt`, `timeZone` | Scheduler, calendar feed |
| `item.due_soon` / `item.overdue` | `dueAt`, `thresholdSpec` | Reminders, automation |
| `item.archived` / `.trashed` / `.restored` / `.purged` | Lifecycle | Cleanup, media GC |
| `bucket.created` / `.updated` / `.reordered` / `.deleted` | A bucket snapshot; `.updated` and `.reordered` add a `changeSet`, `.deleted` says where its items went | Kanban clients, automation, search |
| `label.created` / `.updated` / `.deleted` | A label snapshot; `.updated` adds a `changeSet` | Automation, search |
| `comment.created` / `.updated` / `.deleted` | The comment | Notification, automation |
| `attachment.added` / `.removed` | A media reference | Media GC |
| `recurrence.occurrence_created` | `sourceItemId`, `newItemId`, `occurrenceAt` | History |
| `jumble.entry_received` / `.entry_converted` | The entry, the target item | Automation, AI suggestions |
| `template.instantiated` | `templateId`, `rootItemId` | History |
| `automation.rule_run_started` / `.rule_run_finished` / `.rule_run_failed` | Run details | Monitoring, UI |
| `tenant.provisioned` / `.suspended` / `.deleted` | The tenant | Control plane, metering |
| `membership.granted` / `.revoked` | Scope, role | Audit, notification |

`container.unarchived` is separate from `.restored`, which belongs to the trash: a rule written to
react to something coming back from a deletion must not fire when somebody unarchives a hub. The same
reasoning separates `item.reopened` from `item.completed`, and `bucket.reordered` from
`bucket.updated` — dragging a column one place to the left is not renaming it.

`item.label_added` and `item.label_removed` carry a reference rather than a snapshot, which is the
one exception to the rule above. A label set merges as an OR-set rather than by last writer wins
(offline-sync.md §4.2), so a snapshot of the item would carry a set another device may already have
merged differently; `labelId` is what a rule reacts to and `itemId` is what it reads the rest from.

**Compatibility rules:** fields may only be added; removing or reinterpreting one requires a `.v2`
alongside continued delivery of `.v1` for at least two minor releases.

---

## 5. Use case catalogue (application layer)

Every use case is a `Command`/`Query` struct plus a handler, and is registered in **three**
channels: REST, an MCP tool, and an automation action (see arc42 §4). An extract — the list is the
implementation backlog:

**Work management**
`CreateContainer`, `RenameContainer`, `UpdateContainerPolicies`, `MoveContainer`, `ArchiveContainer`,
`UnarchiveContainer`, `TrashContainer`, `RestoreContainer`,
`CreateWorkItem`, `UpdateWorkItem`, `MoveWorkItem`, `ReorderWorkItem`, `CompleteWorkItem`,
`ReopenWorkItem`, `SetDueDate`, `ClearDueDate`, `AssignWorkItem`, `AutoAssignWorkItem`,
`AddMember`, `RemoveMember`, `AddLabel`, `RemoveLabel`, `SetCover`, `ClearCover`, `SetNotes`,
`SetCustomField`, `ArchiveWorkItem`, `TrashWorkItem`, `RestoreWorkItem`, `PurgeWorkItem`,
`DuplicateWorkItem` (with or without the subtree), `BulkUpdateWorkItems`.

**Structure** `CreateBucket`, `UpdateBucket`, `ReorderBucket`, `DeleteBucket`,
`CreateLabel`, `UpdateLabel`, `DeleteLabel`, `DefineCustomField`, `UpdateCustomField`, `DeleteCustomField`.

**Collaboration** `AddComment`, `EditComment`, `DeleteComment`, `ListActivity`.

**Scheduling** `CreateReminder`, `UpdateReminder`, `DeleteReminder`, `SetRecurrence`,
`UpdateRecurrence`, `RemoveRecurrence`, `SkipOccurrence`, `MaterializeOccurrences` (internal).

**Templates** `CreateTemplate`, `UpdateTemplate`, `DeleteTemplate`, `InstantiateTemplate`.

**Views & query** `QueryItems` (the query DSL), `CreateSavedView`, `UpdateSavedView`, `DeleteSavedView`,
`ShareSavedView`, `ExportView` (CSV/JSON/ICS).

**Jumble** `SubmitJumbleEntry`, `ConvertJumbleEntry`, `DismissJumbleEntry`, `SuggestFromJumbleEntry` (AI, optional).

**Lifecycle** `ListTrash`, `EmptyTrash`, `ListArchive`, `RunRetention` (internal).

**Automation** `CreateRule`, `UpdateRule`, `EnableRule`, `DisableRule`, `DeleteRule`, `TestRule` (dry run),
`TriggerRuleManually`, `ListRuleRuns`, `ReplayRuleRun`.

**Integration** `CreateWebhookSubscription`, `UpdateWebhookSubscription`, `DeleteWebhookSubscription`,
`ListDeliveries`, `ReplayDelivery`, `CreateCalendarFeed`, `RevokeCalendarFeed`,
`CreateAccessToken`, `RevokeAccessToken`, `CreateServiceAccount`.

**Identity & tenancy** `ProvisionTenant`, `UpdateTenantSettings`, `SuspendTenant`, `DeleteTenant`,
`ExportTenantData`, `InviteAccount`, `UpdateAccountPreferences`, `GrantMembership`, `RevokeMembership`,
`CreateGroup`, `UpdateGroup`, `DeleteGroup`.

**Search** `SearchItems` (full text, optionally semantic).

---

## 6. Persistence sketch

The complete DDL: [`../../db/schema.sql`](../../db/schema.sql). The principles:

* Every business table begins with `tenant_id uuid NOT NULL` and carries an RLS policy.
* Composite indices matching the query patterns of the views:
  `(tenant_id, collection_id, bucket_id, order_key)`, `(tenant_id, collection_id, due_at)`,
  `(tenant_id, assignee_id, is_completed, due_at)`, `(tenant_id, path text_pattern_ops)`.
* Partial indices for "not deleted / not archived" (the most common filter).
* Set relations (labels, members) as join tables rather than JSON arrays — filterability takes
  precedence.
* `custom_fields` as `jsonb` with a GIN index; validation happens in the domain code.
* Full text: a generated `tsvector` column with a language-dependent configuration per item.
* Trash and archive as timestamps rather than separate tables (restoring without moving data).
