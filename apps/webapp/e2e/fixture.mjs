// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A workspace the shell's walks share: one hub, one collection with entries, labels, buckets and
// members, and the manifest that declares them - stubbed at the network edge, the way the
// engines walk stubs its own. Nothing here is a claim about the product; it is the least a
// collection can be and still show every control the inventory names.
export const ACCOUNT = { id: '01a0e2e0-0000-7000-8000-000000000001', kind: 'USER', display_name: 'Jérôme Winkel', email: 'j@example.invalid', status: 'ACTIVE', locale: 'en', onboarding_completed_at: '2026-09-01T00:00:00Z' };
export const OTHER = { id: '01a0e2e0-0000-7000-8000-000000000011', kind: 'USER', display_name: 'Mara Lind', email: 'm@example.invalid', status: 'ACTIVE', locale: 'en' };
/** Somebody invited who has not arrived yet: the row an invitation is renewed from (UC-ID-14 check 5). */
export const INVITED = { id: '01a0e2e0-0000-7000-8000-000000000012', kind: 'USER', display_name: 'Bert', email: 'bert@example.invalid', status: 'INVITED' };
export const HUB = { id: '01a0e2e0-0000-7000-8000-000000000002', type: 'HUB', parent_id: null, name: 'House', order_key: 'a0', version: 1 };
export const COLLECTION = { id: '01a0e2e0-0000-7000-8000-000000000003', type: 'COLLECTION', parent_id: HUB.id, name: 'Kitchen', description: 'The renovation, room by room.', order_key: 'a0', version: 1 };
/** One that was put aside: out of the tree, and the only thing the archive screen should find. */
/** Where an arrival may become work for the signed-in person (`GET /jumble/destinations`). */
export const DESTINATIONS = [{ id: COLLECTION.id, name: COLLECTION.name, hub_id: HUB.id, hub_name: HUB.name, hub_private: false }];
export const ARCHIVED = { id: '01a0e2e0-0000-7000-8000-000000000004', type: 'COLLECTION', parent_id: HUB.id, name: 'Last winter', order_key: 'a1', version: 1, archived_at: '2026-08-01T09:00:00Z' };
/**
 * A hub archived in its own right and a collection under it that is not: read-only from above, the
 * case where nothing on the collection's page can be unarchived and every reason names the hub.
 * Neither is in the tree, as the archive leaves them out of it.
 */
export const ARCHIVED_HUB = { id: '01a0e2e0-0000-7000-8000-000000000006', type: 'HUB', parent_id: null, name: 'Old flat', order_key: 'a2', version: 1, archived_at: '2026-07-01T09:00:00Z', effective_archived: true };
export const UNDER_ARCHIVED = { id: '01a0e2e0-0000-7000-8000-000000000007', type: 'COLLECTION', parent_id: ARCHIVED_HUB.id, name: 'Moving boxes', order_key: 'a0', version: 1, effective_archived: true };
export const LABELS = [
  { id: '01a0e2e0-0000-7000-8000-000000000005', name: 'Materials', color_token: 'teal' },
  { id: '01a0e2e0-0000-7000-8000-000000000006', name: 'Urgent', color_token: 'red' },
];
export const BUCKETS = [
  { id: '01a0e2e0-0000-7000-8000-000000000021', collection_id: COLLECTION.id, name: 'To do', order_key: 'a0', wip_limit: null, is_done_bucket: false, version: 1 },
  { id: '01a0e2e0-0000-7000-8000-000000000022', collection_id: COLLECTION.id, name: 'Doing', order_key: 'a1', wip_limit: 2, is_done_bucket: false, version: 1 },
  { id: '01a0e2e0-0000-7000-8000-000000000023', collection_id: COLLECTION.id, name: 'Done', order_key: 'a2', wip_limit: null, is_done_bucket: true, version: 1 },
];
const item = (n, title, extra = {}) => ({ id: `01a0e2e0-0000-7000-8000-0000000000${n}`, type: 'TASK', title, collection_id: COLLECTION.id, parent_id: null, depth: 0, order_key: `a${n}`, version: 1, completion: { is_completed: false }, labels: [], label_ids: [], ...extra });

/**
 * A date this many days from today, as the one dated entry carries.
 *
 * Relative, and it has to be: the timeline opens on a window around *today*, so a fixture pinned to
 * a calendar date is a fixture that expires: the day the window passes it, the drag test goes red
 * on every branch at once — nothing to do with the diff that happens to be in flight.
 * Two days out rather than zero, so the entry sits inside the window on any scale without landing
 * on the marker for today.
 */
const inDays = (days) => new Date(Date.now() + days * 86_400_000).toISOString().slice(0, 10);
export const ITEMS = [
  item('31', 'Order the tiles for the splashback and the floor', { label_ids: [LABELS[0].id], labels: [LABELS[0]], bucket_id: BUCKETS[0].id }),
  item('32', 'Book the electrician', { label_ids: [LABELS[1].id], labels: [LABELS[1]], bucket_id: BUCKETS[1].id, due_at: `${inDays(2)}T09:00:00Z` }),
  item('33', 'Measure the worktop', { bucket_id: BUCKETS[0].id }),
  item('34', 'Choose the paint', { bucket_id: null }),
  item('35', 'Clear the room', { bucket_id: BUCKETS[2].id, completion: { is_completed: true } }),
];
/**
 * The subtree under the first task: two work packages, one of them holding two activities, and an
 * activity of the task's own beside them (domain-model.md §2).
 */
export const CHILDREN = {
  [ITEMS[0].id]: [
    { ...item('41', 'Tiles for the floor', { type: 'WORK_PACKAGE', parent_id: ITEMS[0].id, depth: 1 }) },
    { ...item('42', 'Tiles for the splashback', { type: 'WORK_PACKAGE', parent_id: ITEMS[0].id, depth: 1, completion: { is_completed: true } }) },
    { ...item('43', 'Call the tiler', { type: 'ACTIVITY', parent_id: ITEMS[0].id, depth: 1 }) },
  ],
  '01a0e2e0-0000-7000-8000-000000000041': [
    { ...item('51', 'Measure the floor', { type: 'ACTIVITY', parent_id: '01a0e2e0-0000-7000-8000-000000000041', depth: 2, completion: { is_completed: true } }) },
    { ...item('52', 'Order the floor tiles', { type: 'ACTIVITY', parent_id: '01a0e2e0-0000-7000-8000-000000000041', depth: 2 }) },
  ],
  '01a0e2e0-0000-7000-8000-000000000042': [],
};
/**
 * A grocery list on *List* (UC-WRK-25): one level, amounts, a name for its done entries, and three
 * labels it groups by, in the shop's order. Reached by its address, not listed in the hub, so the
 * walks that count the hub's collections are not disturbed. Its done entries are relative to now:
 * two ticked minutes ago (*In the trolley*), two days ago and earlier (*Recently*, by use).
 */
export const GROCERY_LABELS = [
  { id: '01a0e2e0-0000-7000-8000-000000000081', name: 'Produce', color_token: 'label.green' },
  { id: '01a0e2e0-0000-7000-8000-000000000082', name: 'Dairy', color_token: 'label.blue' },
  { id: '01a0e2e0-0000-7000-8000-000000000083', name: 'Bakery', color_token: 'label.amber' },
];
const listFields = (categories) => ({ TASK: { capabilities: ['COMPLETION', 'LABELS', 'HISTORY', 'CUSTOM_FIELDS', 'AMOUNT'], categories } });
/** The structure in force of a list, as a container read answers it; `figure` is UX-24's. */
export const LIST_STRUCTURE = {
  preset: 'LIST',
  settings: {
    levels: [{ type: 'TASK', name: null, done_name: 'In the trolley' }],
    fields: listFields(GROCERY_LABELS.map((label) => label.id)),
    views: { offered: ['LIST_COLLAPSED'], default: 'LIST_COLLAPSED' },
    counts: { figure: 'OPEN', field_key: null },
    completion: 'MANUAL',
    capture: ['LINES', 'AMOUNT'],
    celebration: 'SUBTLE',
    automation: [],
  },
  sources: {},
  locks: {},
};
export const GROCERIES = { id: '01a0e2e0-0000-7000-8000-000000000060', type: 'COLLECTION', parent_id: HUB.id, name: 'Groceries', order_key: 'a2', version: 1, structure: LIST_STRUCTURE };
const minutesAgo = (minutes) => new Date(Date.now() - minutes * 60_000).toISOString();
const grocery = (n, title, extra = {}) => ({ ...item(n, title, extra), collection_id: GROCERIES.id, completion_count: 0, amount: null, ...extra });
export const GROCERY_ITEMS = [
  grocery('71', 'apples', { amount: '1 kg', label_ids: [GROCERY_LABELS[0].id] }),
  grocery('72', 'milk', { amount: '2', label_ids: [GROCERY_LABELS[1].id], completion_count: 6 }),
  grocery('73', 'bread', { label_ids: [GROCERY_LABELS[2].id] }),
  grocery('74', 'salt'),
  grocery('75', 'butter', { label_ids: [GROCERY_LABELS[1].id], completion: { is_completed: true, completed_at: minutesAgo(2) }, completion_count: 3 }),
  grocery('76', 'coffee', { amount: '500 g', completion: { is_completed: true, completed_at: minutesAgo(2 * 24 * 60) }, completion_count: 2 }),
  grocery('77', 'oat milk', { completion: { is_completed: true, completed_at: minutesAgo(9 * 24 * 60) }, completion_count: 8 }),
  grocery('78', 'tea', { completion: { is_completed: true, completed_at: minutesAgo(3 * 24 * 60) }, completion_count: 2 }),
];
const GROCERIES_AT_START = GROCERY_ITEMS.map((entry) => ({ ...entry }));
/** Puts the grocery list back as it started: a walk that ticks things leaves the next one alone. */
export function resetGroceries() {
  GROCERY_ITEMS.splice(0, GROCERY_ITEMS.length, ...GROCERIES_AT_START.map((entry) => ({ ...entry })));
}
/**
 * An entry archived in its own right with an activity under it that is not - the server's model:
 * archiving freezes the entry alone (ArchiveWorkItem.go) - and an entry in the archived collection.
 * Neither is in `ITEMS`, so no list walk counts them; their pages are reached by address.
 */
export const ARCHIVED_ENTRY = item('61', 'Old shelving plan', { archived_at: '2026-08-15T09:00:00Z' });
export const UNDER_ARCHIVED_ENTRY = item('62', 'Measure the alcove', { type: 'ACTIVITY', parent_id: ARCHIVED_ENTRY.id, depth: 1 });
CHILDREN[ARCHIVED_ENTRY.id] = [UNDER_ARCHIVED_ENTRY];
export const IN_ARCHIVED = item('63', 'Insulate the loft', { collection_id: ARCHIVED.id });
export const ALL_ITEMS = [...ITEMS, ...Object.values(CHILDREN).flat(), ARCHIVED_ENTRY, IN_ARCHIVED, ...GROCERY_ITEMS];

const ALL = { read: 'ALL', create: 'ALL', change: 'ALL', comment: 'ALL' };
export const ROLES = [
  { role: 'OWNER', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'AUTOMATION', 'DELETE_CONTAINER', 'READ_CONFIGURATION', 'AUDIT_READ'], item_access: ALL },
  { role: 'ADMIN', permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'AUTOMATION', 'READ_CONFIGURATION', 'AUDIT_READ'], item_access: ALL },
  { role: 'MEMBER', permissions: ['READ', 'WRITE_ITEMS', 'AUTOMATION'], item_access: ALL },
  { role: 'CONTRIBUTOR', permissions: ['READ', 'WRITE_ITEMS'], item_access: { read: 'ALL', create: 'ALL', change: 'ASSIGNED', comment: 'ASSIGNED' } },
  { role: 'VIEWER', permissions: ['READ'], item_access: { read: 'ALL', create: 'NONE', change: 'NONE', comment: 'NONE' } },
  { role: 'GUEST', permissions: ['READ'], item_access: { read: 'ALL', create: 'NONE', change: 'NONE', comment: 'ALL' } },
  { role: 'AUDITOR', permissions: ['READ_CONFIGURATION', 'AUDIT_READ'], item_access: { read: 'NONE', create: 'NONE', change: 'NONE', comment: 'NONE' } },
];

export const MANIFEST = {
  product_version: '0.9.0', api_version: 'v1', tenancy_mode: 'single',
  // The three profiles as `domain-model.md` §2 defines them and a real server answers them,
  // spelling included — the fixture said `ASSIGNEE` for `ASSIGNMENT` and gave a work package two
  // capabilities of fourteen, which is a fake that agrees with no installation.
  item_types: [
    { type: 'TASK', capabilities: ['COMPLETION', 'DUE_DATE', 'REMINDER', 'ASSIGNMENT', 'MEMBERS', 'BUCKET', 'NOTES', 'LABELS', 'COMMENTS', 'COVER', 'ATTACHMENTS', 'HISTORY', 'RECURRENCE', 'CUSTOM_FIELDS', 'AMOUNT'], allowed_child_types: ['WORK_PACKAGE', 'ACTIVITY'], max_depth: 3 },
    { type: 'WORK_PACKAGE', capabilities: ['COMPLETION', 'DUE_DATE', 'REMINDER', 'ASSIGNMENT', 'MEMBERS', 'NOTES', 'LABELS', 'COMMENTS', 'ATTACHMENTS', 'HISTORY', 'CUSTOM_FIELDS'], allowed_child_types: ['ACTIVITY'], max_depth: 2 },
    { type: 'ACTIVITY', capabilities: ['COMPLETION', 'DUE_DATE', 'REMINDER', 'ASSIGNMENT', 'HISTORY'], allowed_child_types: [], max_depth: 1 },
  ],
  supported_locales: [{ locale: 'en', direction: 'ltr' }, { locale: 'de', direction: 'ltr' }],
  query_fields: [
    { field: 'title', operators: ['CONTAINS', 'EQ'], sortable: true, groupable: false, nullable: false },
    { field: 'due_at', operators: ['LT', 'GT', 'IS_NULL'], sortable: true, groupable: false, nullable: true },
    { field: 'bucket_id', operators: ['EQ', 'IN'], sortable: false, groupable: true, nullable: true },
  ],
  view_layouts: ['LIST_COLLAPSED', 'LIST_EXPANDED', 'KANBAN', 'TIMELINE'],
  // The role matrix as the server publishes it (domain-model.md §3.2), strongest first - the order
  // the effective role along a path is taken in, and the per-entry cells the screens offer from.
  roles: ROLES,
  limits: { max_bulk_operations: 100 }, features: {},
  // What the reader - the workspace's owner - holds on the workspace itself, as the server answers
  // a signed-in caller. A walk as another role answers `PERMISSIONS[role]` instead.
  workspace_permissions: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'AUTOMATION', 'DELETE_CONTAINER', 'READ_CONFIGURATION', 'AUDIT_READ'],
  // Whether this reader may create a private hub of their own: a person holding a role, where the
  // workspace allows private hubs and holds more than one person - the fixture's workspace does.
  private_hub_offered: true,
};

/**
 * Rule drafting (UC-AUT-10): a service account a rule may run as, the vocabulary the rule editor
 * offers, and the draft a sentence comes back as - about the reader's own account, with what it
 * could not draft. `AI_MANIFEST` is the workspace with AI available; the shared `MANIFEST` keeps it
 * off, so every other walk sees no AI control (UC-AI-01 check 1).
 */
export const SERVICE_ACCOUNT = { id: '01a0e2e0-0000-7000-8000-0000000000a5', display_name: 'Automation', kind: 'SERVICE' };
export const DRAFT_JOB = '01a0e2e0-0000-7000-8000-0000000007c1';
export const RULE_DRAFT_ID = '01a0e2e0-0000-7000-8000-0000000007c2';
export const DRAFTED_RULE_ID = '01a0e2e0-0000-7000-8000-0000000007c3';
export const AUTOMATION = {
  triggers: ['EVENT', 'SCHEDULE', 'RELATIVE_DATE', 'INBOUND_WEBHOOK', 'MANUAL', 'JUMBLE_ENTRY'],
  actions: ['ADD_LABEL', 'ASSIGN_WORK_ITEM', 'COMPLETE_WORK_ITEM', 'DESCRIBE_RULE'],
  action_fields: {
    ADD_LABEL: [{ name: 'item_id', kind: 'id', required: true }, { name: 'label_id', kind: 'id', required: true }],
    ASSIGN_WORK_ITEM: [{ name: 'account_id', kind: 'id', required: true }, { name: 'item_id', kind: 'id', required: true }, { name: 'expected_version', kind: 'integer', required: false, rule: false }],
    COMPLETE_WORK_ITEM: [{ name: 'item_id', kind: 'id', required: true }],
    DESCRIBE_RULE: [{ name: 'scope', kind: 'object', required: true }, { name: 'description', kind: 'string', required: true }],
  },
  action_summaries: {},
};

/** The draft a sentence comes back as: made now, so it is newer than any ask a walk makes. */
export function ruleDraft(status = 'PROPOSED') {
  return {
    id: RULE_DRAFT_ID, target_type: 'ACCOUNT', target_id: '01a0e2e0-0000-7000-8000-000000000001', kind: 'RULE', status,
    payload: {
      scope: { type: 'TENANT' },
      trigger: { kind: 'EVENT', event_type: 'de.hubtask.work.item.created.v1' },
      conditions: [{ expr: "item.type == 'TASK'" }],
      actions: [{ kind: 'ASSIGN_WORK_ITEM', params: { account_id: '01a0e2e0-0000-7000-8000-000000000011' } }],
      not_drafted: [{ part: 'set it due in two days' }],
    },
    source: 'AI', model: 'local-model-7b', prompt_id: 'draft-rule', prompt_version: 'v1',
    produced_at: new Date().toISOString(), created_at: new Date().toISOString(), version: 1, dropped_nodes: 0,
  };
}

/** The rule saving the draft created: switched off, as every new rule is. */
export const DRAFTED_RULE = {
  id: DRAFTED_RULE_ID, name: 'When a task is created, assign it', scope: { type: 'TENANT' }, enabled: false,
  run_as: SERVICE_ACCOUNT.id, trigger: { kind: 'EVENT', event_type: 'de.hubtask.work.item.created.v1' },
  conditions: [{ expr: "item.type == 'TASK'" }],
  actions: [{ kind: 'ASSIGN_WORK_ITEM', params: { account_id: '01a0e2e0-0000-7000-8000-000000000011' } }],
  on_error: 'STOP', failure_count: 0, version: 1, findings: [], last_run: null,
};

/** The workspace with AI available and the rule editor's vocabulary: what a drafting walk opens on. */
export const AI_MANIFEST = {
  ...MANIFEST,
  features: { ...MANIFEST.features, ai_suggestions: true },
  event_types: ['de.hubtask.work.item.created.v1', 'de.hubtask.work.item.overdue.v1'],
  automation: AUTOMATION,
};

/**
 * What each role holds on the workspace when that is where it was granted - the role matrix of
 * `domain-model.md` §3.2, as `workspace_permissions` answers it. `BELOW` is somebody whose roles sit
 * only on a hub, a collection or an entry.
 */
export const PERMISSIONS = {
  OWNER: MANIFEST.workspace_permissions,
  ADMIN: ['READ', 'WRITE_ITEMS', 'STRUCTURE', 'MANAGE_MEMBERS', 'AUTOMATION', 'READ_CONFIGURATION', 'AUDIT_READ'],
  MEMBER: ['READ', 'WRITE_ITEMS', 'AUTOMATION'],
  CONTRIBUTOR: ['READ', 'WRITE_ITEMS'],
  VIEWER: ['READ'],
  GUEST: ['READ'],
  AUDITOR: ['READ_CONFIGURATION', 'AUDIT_READ'],
  BELOW: [],
};
export const PAGE = { data: [], items: [], page: { next_cursor: null, has_more: false } };
/**
 * What the server answers a workspace owner may grant at any scope (`grantable_roles`): every role.
 * A walk as an administrator answers `ADMIN_GRANTS`, which leaves out OWNER.
 */
export const OWNER_GRANTS = ['OWNER', 'ADMIN', 'MEMBER', 'CONTRIBUTOR', 'VIEWER', 'GUEST', 'AUDITOR'];
export const ADMIN_GRANTS = OWNER_GRANTS.slice(1);

/** One sign-in rule as the contract answers it: nobody above decided it, nothing locks it. */
const policyRule = (value) => ({ value, installation: value, lock: null, source: 'DEFAULT', installation_source: 'DEFAULT' });

/**
 * The workspace's sign-in rules at Hubtask's defaults, so *Administration → Sign-in* can be walked
 * (UC-ID-12): every choice offered, since the installation requires nothing.
 */
export const SIGN_IN_POLICY = {
  password: {
    min_length: policyRule(12), min_lowercase: policyRule(0), min_uppercase: policyRule(0), min_digits: policyRule(0),
    min_symbols: policyRule(0), min_classes: policyRule(0), max_repeat: policyRule(0),
    common_passwords: policyRule(true), context_words: policyRule(true), breach_check: policyRule(false),
    max_age_days: policyRule(0), history_count: policyRule(0), min_age_hours: policyRule(0),
  },
  mfa_required_for: policyRule('NONE'),
  methods: policyRule(['PASSWORD']),
  session: { max_days: policyRule(30), idle_minutes: policyRule(0) },
  legal: {
    imprint_url: policyRule(''), privacy_url: policyRule(''), terms_url: policyRule(''), accessibility_url: policyRule(''),
    terms_version: policyRule(''),
  },
  rotation_from: null,
};

/**
 * The workspace as `GET /tenant` answers it: two people, so a private hub is offered (UC-ID-16
 * check 8). A walk about a workspace of one routes `/tenant` itself.
 */
export const WORKSPACE = { id: '01a0e2e0-0000-7000-8000-0000000005a1', slug: 'house', display_name: 'House', status: 'ACTIVE', default_locale: 'en', default_time_zone: 'UTC', require_admin_totp: false, created_at: '2026-08-01T08:00:00Z', version: 1, private_hubs_offered: true, several_people: true, sign_in_policy: SIGN_IN_POLICY };

/**
 * The private hubs as an administrator lists them: one in use, owned by the other person, and one
 * whose last member left, in the trash. No names - the list has none to give. The reader is the
 * workspace's owner and holds nothing in either, so the emergency access is offered on both
 * (ADR-0073 §4).
 */
/** This installation's own callback, as the presets answer it. */
export const REDIRECT_URI = 'https://hubtask.example/api/v1/auth/oidc/callback';

const preset = (kind, provisioning, extra = {}) => ({
  kind, scopes: ['openid', 'email', 'profile'], addresses_verified: kind !== 'GENERIC',
  public: kind === 'GOOGLE', provisioning, installation_provisioning: ['INVITED_ONLY'],
  redirect_uri: REDIRECT_URI, instructions: `identity_provider.preset.${kind.toLowerCase()}.instructions`,
  supports_templated_issuer: kind === 'MICROSOFT',
  shared_issuer_paths: kind === 'MICROSOFT' ? ['common', 'organizations'] : [],
  personal_issuer_paths: kind === 'MICROSOFT' ? ['common', 'consumers', '9188040d-6c67-4c5b-b112-36a304b66dad'] : [],
  personal_directories: kind === 'MICROSOFT' ? ['9188040d-6c67-4c5b-b112-36a304b66dad'] : [],
  ...extra,
});

/** The three presets as `GET /identity-provider-presets` answers them today (UC-ID-11). */
export const PROVIDER_PRESETS = [
  preset('GOOGLE', ['INVITED_ONLY', 'DOMAINS'], {
    issuer: 'https://accounts.google.com', directory_claim: 'hd', particular: 'identity_provider.preset.google.particular',
  }),
  // A preset with a directory claim is offered to every workspace with the same modes it has for
  // one (the installation's list is narrower only without a claim), as the server answers it.
  preset('MICROSOFT', ['INVITED_ONLY', 'DOMAINS', 'ANY'], { directory_claim: 'tid', installation_provisioning: ['INVITED_ONLY', 'DOMAINS', 'ANY'] }),
  preset('GENERIC', ['INVITED_ONLY', 'DOMAINS', 'ANY']),
];

export const PRIVATE_HUBS = [
  { id: '01a0e2e0-0000-7000-8000-0000000005b1', owners: [OTHER.id], collections: 2, entries: 14, attachment_bytes: 2_621_440, created_at: '2026-09-02T08:00:00Z', trashed_at: null, purge_on: null, emergency_access: { offered: true, open_until: null } },
  { id: '01a0e2e0-0000-7000-8000-0000000005b2', owners: [], collections: 1, entries: 3, attachment_bytes: 0, created_at: '2026-09-03T08:00:00Z', trashed_at: '2026-10-01T08:00:00Z', purge_on: '2026-10-31', emergency_access: { offered: true, open_until: null } },
];

/**
 * An emergency access to the first private hub: the other person's hub, opened by the reader, the
 * workspace's owner. What the members are told, and what the hub's page says.
 */
export const EMERGENCY_ACCESS = { id: '01a0e2e0-0000-7000-8000-0000000005d1', hub_id: PRIVATE_HUBS[0].id, opened_by: ACCOUNT.id, reason: 'Mara is in hospital and the insurance papers are in here.', opened_at: '2026-10-12T14:30:00Z', ends_at: '2026-10-12T15:30:00Z' };

/**
 * People's requests: one that can still be extended (the server answers its bound), one extended
 * once, one installation-wide - the operator's, which the screen offers nothing on - an erasure
 * waiting to be started, in full, and one a legal hold kept part of.
 */
export const PRIVACY_REQUESTS = [
  { id: '01a0e2e0-0000-7000-8000-0000000000a1', kind: 'ACCESS', status: 'RECEIVED', scope: 'TENANT', subject_email: 'open@example.invalid', received_at: '2026-09-11T08:00:00Z', due_at: '2026-10-11T08:00:00Z', extendable_until: '2026-12-11' },
  { id: '01a0e2e0-0000-7000-8000-0000000000a2', kind: 'PORTABILITY', status: 'IN_PROGRESS', scope: 'TENANT', subject_email: 'extended@example.invalid', received_at: '2026-09-01T08:00:00Z', due_at: '2026-11-20T22:59:59Z', original_due_at: '2026-10-01T08:00:00Z', extension_reason: 'NUMBER_OF_REQUESTS', informed_on: '2026-09-15' },
  { id: '01a0e2e0-0000-7000-8000-0000000000a3', kind: 'ACCESS', status: 'RECEIVED', scope: 'INSTALLATION', subject_email: 'everywhere@example.invalid', received_at: '2026-09-11T08:00:00Z', due_at: '2026-10-12T08:00:00Z', extendable_until: '2026-12-11' },
  { id: '01a0e2e0-0000-7000-8000-0000000000a4', kind: 'ERASURE', status: 'RECEIVED', scope: 'TENANT', subject_email: 'erase@example.invalid', erasure_mode: 'FULL_DELETE', received_at: '2026-09-11T08:00:00Z', due_at: '2026-10-13T08:00:00Z' },
  { id: '01a0e2e0-0000-7000-8000-0000000000a5', kind: 'ERASURE', status: 'COMPLETED', scope: 'TENANT', subject_email: 'kept@example.invalid', erasure_mode: 'FULL_DELETE', received_at: '2026-09-01T08:00:00Z', due_at: '2026-10-01T08:00:00Z', completed_at: '2026-09-05T08:00:00Z', kept: [{ hold_id: '01a0e2e0-0000-7000-8000-0000000003a1', hold_scope: { kind: 'CONTAINER', id: HUB.id }, account: false, entries: 0, comments: 2, assignments: 1, intake: 0 }], kept_legal_basis: 'ART_17_3_E' },
  // A hold released, and the rest waiting: the person is the workspace's only owner (UC-PRV-03 check 14).
  { id: '01a0e2e0-0000-7000-8000-0000000000a6', kind: 'ERASURE', status: 'COMPLETED', scope: 'TENANT', subject_email: 'onlyowner@example.invalid', erasure_mode: 'ANONYMIZE', received_at: '2026-09-02T08:00:00Z', due_at: '2026-10-02T08:00:00Z', completed_at: '2026-09-06T08:00:00Z', kept: [{ hold_id: '01a0e2e0-0000-7000-8000-0000000003a2', hold_scope: { kind: 'ACCOUNT', id: OTHER.id }, account: true, entries: 0, comments: 0, assignments: 0, intake: 0, blocked: { code: 'memberships.last_owner' } }], kept_legal_basis: 'ART_17_3_E' },
];

/** Two holds in force: one on the hub, one on a person. */
export const LEGAL_HOLDS = [
  { id: '01a0e2e0-0000-7000-8000-0000000003a1', reason: 'Pending litigation, ref. 4 O 128/26', placed_by: ACCOUNT.id, placed_at: '2026-09-20T08:00:00Z', scope: { kind: 'CONTAINER', id: HUB.id } },
  { id: '01a0e2e0-0000-7000-8000-0000000003a2', reason: 'Tax inspection 2026', placed_by: ACCOUNT.id, placed_at: '2026-09-21T08:00:00Z', scope: { kind: 'ACCOUNT', id: OTHER.id } },
];

/**
 * The installation's workspaces as the operator lists them: one under a legal hold, one pending
 * deletion under one, one with neither - the three rows a hold changes (data-protection.md §5) -
 * and the operator's own (`WORKSPACE`), whose suspension or deletion ends the operator's session.
 */
export const WORKSPACES = [
  { id: WORKSPACE.id, slug: WORKSPACE.slug, display_name: WORKSPACE.display_name, status: 'ACTIVE', default_locale: 'en', default_time_zone: 'UTC', created_at: WORKSPACE.created_at, purge_after: null, legal_hold: false },
  { id: '01a0e2e0-0000-7000-8000-0000000004a1', slug: 'acme', display_name: 'Acme GmbH', status: 'ACTIVE', default_locale: 'de', default_time_zone: 'Europe/Berlin', created_at: '2026-08-01T08:00:00Z', purge_after: null, legal_hold: true },
  { id: '01a0e2e0-0000-7000-8000-0000000004a2', slug: 'leaving', display_name: 'Leaving Ltd', status: 'PENDING_DELETION', default_locale: 'en', default_time_zone: 'UTC', created_at: '2026-08-02T08:00:00Z', purge_after: '2026-09-30T08:00:00Z', legal_hold: true },
  { id: '01a0e2e0-0000-7000-8000-0000000004a3', slug: 'household', display_name: 'Household', status: 'ACTIVE', default_locale: 'en', default_time_zone: 'UTC', created_at: '2026-08-03T08:00:00Z', purge_after: null, legal_hold: false },
];

/**
 * The AI models the installation offers, as the operator lists them: one, with a key, in the EEA.
 */
export const OFFERED_AI = [
  { id: '01a0e2e0-0000-7000-8000-0000000006a1', display_name: 'Large (EU)', processor: 'Example AI SAS', kind: 'OPENAI_COMPATIBLE', base_url: 'https://models.example.eu/v1', completion_model: 'large-2', embedding_model: 'embed-1', has_api_key: true, jurisdiction: 'EEA', created_at: '2026-10-01T08:00:00Z', version: 1 },
];

/**
 * A workspace's AI source on a private installation (D1): its own model, nothing offered, no lock -
 * today's screen. A walk of an installation that offers a model routes `/ai-source` itself.
 */
export const AI_SOURCE = { source: 'OWN', sources: 'OWN', installation_sources: 'OWN', sources_origin: 'DEFAULT', min_jurisdiction: 'ANY', min_jurisdiction_origin: 'DEFAULT', offered: [], search_rebuilding: false, version: 0, person_opt_out: 'OFFERED', person_opt_out_origin: 'DEFAULT', keeping_out_count: 0, prepare_on_arrival: false };

/**
 * A person's own view of keeping their content out of AI where there is nothing to choose: AI off
 * in the fixture's workspace (UC-PRV-05 check 8). A walk of the switch routes it itself.
 */
export const AI_OBJECTION = { shown: false, person_opt_out: 'OFFERED', keeping_out: false, in_effect: false, notice_pending: false };

/** The consent register: one person keeps their content out, recorded by themselves. */
export const CONSENT_REGISTER = { data: [{ account_id: '01a0e2e0-0000-7000-8000-0000000000b7', display_name: 'Kim Keeper', since: '2026-10-05T09:12:00Z', source: 'user' }] };

/**
 * The keyring mid-rotation, as the operator reads it: the current key, a predecessor nothing names
 * any more, and a key the ring no longer holds that a sign-in provider's secret and two webhook
 * secrets still name - the census by kind, never by workspace.
 */
/** A purchase platform's subscription that could not be reached: four events wait (UC-INS-14). */
export const PLATFORM_SUBSCRIPTIONS = [{
  id: '01a0e2e0-0000-7000-8000-0000000007a1',
  target_url: 'https://shop.anbieter.example/hubtask',
  state: 'ACTIVE',
  dead_letters: 4,
  last_delivery_at: new Date(Date.now() - 3 * 3_600_000).toISOString(),
  last_error: 'webhooks.target_unavailable',
  from_file: false,
  created_at: '2026-10-16T08:00:00Z',
  version: 1,
}];

export const ENCRYPTION_STATUS = {
  active_key_id: 'k2',
  keys: [
    { key_id: 'k2', active: true, in_ring: true, sealed_values: 12, stores: [
      { store: 'ai_provider', sealed_values: 2 }, { store: 'webhook_subscription', sealed_values: 10 },
    ] },
    { key_id: 'k1', active: false, in_ring: true, sealed_values: 0, stores: [] },
    { key_id: 'k0', active: false, in_ring: false, sealed_values: 3, stores: [
      { store: 'identity_provider', sealed_values: 1 }, { store: 'webhook_subscription', sealed_values: 2 },
    ] },
  ],
};

/** The passkeys of `ACCOUNT` as *Password and sign-in* reads them (identity.md §23). */
export const PASSKEYS = { passkeys: [], limit: 10, can_add: true, can_remove: true, rp_id: 'localhost' };

/**
 * What counts, one collection per figure (UC-WRK-29): a hub of its own, so the walks that count the
 * rows of the navigation meet the tree they always met. Shown where a walk asks for it
 * (`showFigures`, the figure walk) or the preview is started with `HUBTASK_FIXTURE_FIGURES=1`.
 */
export const FIGURE_HUB = { id: '01a0e2e0-0000-7000-8000-000000000061', type: 'HUB', parent_id: null, name: 'Home', order_key: 'a1', version: 1 };
const today = () => new Date().toISOString().slice(0, 10);
const structureOf = (levels, counts, extra = {}) => ({
  preset: null,
  settings: {
    levels: levels.map((type) => ({ type, name: null })),
    fields: Object.fromEntries(levels.map((type) => [type, { capabilities: MANIFEST.item_types.find((t) => t.type === type).capabilities }])),
    counts,
  },
  sources: { levels: 'COLLECTION', fields: 'COLLECTION', views: 'DEFAULT', counts: 'COLLECTION', completion: 'DEFAULT', capture: 'DEFAULT', celebration: 'DEFAULT', automation: 'DEFAULT' },
  locks: {},
  counts_clipped: null,
  ...extra,
});
const figureCollection = (n, name, levels, counts, figure, extra = {}) => ({
  id: `01a0e2e0-0000-7000-8000-0000000000${n}`, type: 'COLLECTION', parent_id: FIGURE_HUB.id, name, order_key: `a${n}`, version: 1,
  structure: structureOf(levels, counts, extra),
  ...(figure ? { figure: { today: today(), ...figure } } : {}),
});
export const FIGURES = {
  sum: figureCollection('62', 'Groceries', ['TASK'], { figure: 'FIELD_SUM', field_key: 'amount' }, { kind: 'FIELD_SUM', open: 5, sum: 38.1, summed: 4 }),
  due: figureCollection('63', 'Household', ['TASK', 'ACTIVITY'], { figure: 'DUE', field_key: null }, { kind: 'DUE', due: 3 }),
  progress: figureCollection('64', 'Renovation', ['TASK', 'WORK_PACKAGE', 'ACTIVITY'], { figure: 'PROGRESS', field_key: null }, { kind: 'PROGRESS', done: 5, total: 13 }),
  rhythm: figureCollection('65', 'Running', ['TASK'], { figure: 'RHYTHM', field_key: null }, { kind: 'RHYTHM', days: ['2026-10-19', '2026-10-21'], week_start: '2026-10-19' }),
  newWeek: figureCollection('66', 'Reading', ['TASK'], { figure: 'RHYTHM', field_key: null }, { kind: 'RHYTHM', days: [], week_start: '2026-10-19' }),
  open: figureCollection('67', 'Shed', ['TASK'], { figure: 'OPEN', field_key: null }, { kind: 'OPEN', open: 7 }, {
    sources: { levels: 'COLLECTION', fields: 'COLLECTION', views: 'DEFAULT', counts: 'HUB', completion: 'DEFAULT', capture: 'DEFAULT', celebration: 'DEFAULT', automation: 'DEFAULT' },
    counts_clipped: { from: { figure: 'PROGRESS', field_key: null }, reason: 'ONE_LEVEL' },
  }),
  none: figureCollection('68', 'Notes', ['TASK'], { figure: 'NONE', field_key: null }, undefined),
  unavailable: figureCollection('69', 'Archive box', ['TASK'], { figure: 'OPEN', field_key: null }, { kind: 'OPEN', unavailable: true }),
};
/** A number field the sum may name. */
export const AMOUNT_FIELD = { id: '01a0e2e0-0000-7000-8000-000000000070', collection_id: FIGURES.sum.id, key: 'amount', kind: 'NUMBER', options: [], is_required: false, applies_to: ['TASK'], version: 1 };

let figuresShown = typeof process !== 'undefined' && process.env?.HUBTASK_FIXTURE_FIGURES === '1';
/** Adds the figure hub to the tree, for this process. */
export function showFigures() {
  figuresShown = true;
}

/** The figure hub's reads and the structure write; `undefined` where the general stub answers. */
function figureRoute(route, request, url, path) {
  if (!figuresShown) return undefined;
  const collections = Object.values(FIGURES);
  if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') {
    return route.fulfill({ json: { ...PAGE, data: [HUB, FIGURE_HUB] } });
  }
  if (path.endsWith('/api/v1/containers') && url.searchParams.get('parent_id') === FIGURE_HUB.id) {
    return route.fulfill({ json: { ...PAGE, data: collections } });
  }
  if (path.endsWith(`/api/v1/containers/${FIGURE_HUB.id}`)) return route.fulfill({ json: FIGURE_HUB });
  // The replica holds the figure hub's containers as their records carry them: without a figure,
  // which is the server's read alone (offline-sync.md §4.2).
  if (path.endsWith('/api/v1/sync:snapshot')) {
    const containers = [HUB, COLLECTION, FIGURE_HUB, ...collections.map(({ figure, ...rest }) => rest)];
    const records = containers.map((c) => JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: c.id, container_id: c.parent_id, payload: c }));
    return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${records.join('\n')}\n{"cursor":"c-e2e"}\n` });
  }
  const structured = collections.find((c) => path.endsWith(`/api/v1/containers/${c.id}/structure`));
  if (structured && request.method() === 'PATCH') {
    const counts = request.postDataJSON().settings.counts;
    const settings = { ...structured.structure.settings, counts: { field_key: null, ...counts } };
    return route.fulfill({ json: { ...structured, version: structured.version + 1, structure: { ...structured.structure, settings }, kept_by_lock: [], clipped_by_levels: [] } });
  }
  const one = collections.find((c) => path.endsWith(`/api/v1/containers/${c.id}`));
  if (one) return route.fulfill({ json: one });
  if (path.endsWith('/custom-fields') && url.searchParams.get('collection_id') === FIGURES.sum.id) return route.fulfill({ json: [AMOUNT_FIELD] });
  return undefined;
}

/**
 * Milestones and kudos (UC-WRK-29 checks 4-7): a hub of its own, finished, whose one collection is
 * finished too; an entry Mara completed, which the reader may give kudos for, and one the reader
 * completed, which Mara gave kudos for. Shown where a walk asks for it (`showCelebrations`) or the
 * preview is started with `HUBTASK_FIXTURE_CELEBRATIONS=1`; `showCelebrations({ celebrating: false })`
 * answers the account with its celebrations switched off.
 */
const FINISHED_AT = '2026-10-20T16:30:00Z';
export const FINISHED_HUB = { id: '01a0e2e0-0000-7000-8000-0000000025f1', type: 'HUB', parent_id: null, name: 'Garden', order_key: 'a3', version: 1, milestone: { reached_at: FINISHED_AT } };
export const FINISHED = { id: '01a0e2e0-0000-7000-8000-0000000025f2', type: 'COLLECTION', parent_id: FINISHED_HUB.id, name: 'Spring planting', order_key: 'a0', version: 1, milestone: { reached_at: FINISHED_AT } };
const finishedEntry = (n, title, by) => ({
  id: `01a0e2e0-0000-7000-8000-0000000025f${n}`, type: 'TASK', title, collection_id: FINISHED.id, parent_id: null, depth: 0, order_key: `a${n}`, version: 2,
  completion: { is_completed: true, completed_at: FINISHED_AT, completed_by: by }, labels: [], label_ids: [],
});
export const DONE_BY_OTHER = finishedEntry('3', 'Plant the tulips', OTHER.id);
export const DONE_BY_ME = finishedEntry('4', 'Water the beds', ACCOUNT.id);

let celebrationsShown = typeof process !== 'undefined' && process.env?.HUBTASK_FIXTURE_CELEBRATIONS === '1';
let celebrating = true;
let kudosGiven = false;
/** Adds the finished hub to the tree, for this process; nothing given yet. */
export function showCelebrations({ celebrating: on = true } = {}) {
  celebrationsShown = true;
  celebrating = on;
  kudosGiven = false;
}

/** The finished hub's reads and the kudos; `undefined` where the general stub answers. */
function celebrationRoute(route, request, url, path) {
  if (!celebrationsShown) return undefined;
  const method = request.method();
  if (path.endsWith('/api/v1/accounts/me') || path.endsWith(`/api/v1/accounts/${ACCOUNT.id}`)) {
    return route.fulfill({ json: { ...ACCOUNT, celebrations: celebrating } });
  }
  if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') {
    return route.fulfill({ json: { ...PAGE, data: [HUB, FINISHED_HUB] } });
  }
  if (path.endsWith('/api/v1/containers') && url.searchParams.get('parent_id') === FINISHED_HUB.id) {
    return route.fulfill({ json: { ...PAGE, data: [FINISHED] } });
  }
  if (path.endsWith(`/api/v1/containers/${FINISHED_HUB.id}`)) return route.fulfill({ json: FINISHED_HUB });
  if (path.endsWith(`/api/v1/containers/${FINISHED.id}`)) return route.fulfill({ json: FINISHED });
  // The replica holds them as their records carry them: without a milestone, the server's read alone.
  if (path.endsWith('/api/v1/sync:snapshot')) {
    const records = [HUB, COLLECTION, FINISHED_HUB, FINISHED].map(({ milestone, ...c }) => JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: c.id, container_id: c.parent_id, payload: c }));
    return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${records.join('\n')}\n{"cursor":"c-e2e"}\n` });
  }
  if (path.endsWith('/api/v1/items:query') && request.postDataJSON()?.scope?.container_id === FINISHED.id) {
    const data = [DONE_BY_OTHER, DONE_BY_ME];
    return route.fulfill({ json: { data, groups: [], page: { next_cursor: null, has_more: false }, total: data.length } });
  }
  const entry = [DONE_BY_OTHER, DONE_BY_ME].find((each) => path.endsWith(`/api/v1/items/${each.id}`));
  if (entry && method === 'GET') return route.fulfill({ json: entry });
  // Each reader's own view: the reader's toggle on Mara's entry, Mara's kudos on the reader's.
  if (path.endsWith(`/api/v1/items/${DONE_BY_OTHER.id}/kudos`)) {
    if (method === 'PUT') kudosGiven = true;
    if (method === 'DELETE') kudosGiven = false;
    return route.fulfill({ json: { item_id: DONE_BY_OTHER.id, given: kudosGiven, from: [] } });
  }
  if (path.endsWith(`/api/v1/items/${DONE_BY_ME.id}/kudos`)) {
    return route.fulfill({ json: { item_id: DONE_BY_ME.id, given: false, from: [{ account_id: OTHER.id, given_at: '2026-10-21T08:00:00Z' }] } });
  }
  return undefined;
}

/**
 * Places in their own colour and symbol (UC-WRK-33): a hub with a colour and a symbol, a private
 * one with both, and under the first a collection with both and one with neither - plus two dated
 * entries from different collections, so the overview and search mix places. Shown where a walk
 * asks for it (`showLooks`) or the preview is started with `HUBTASK_FIXTURE_LOOKS=1`.
 *
 * It answers writes: a hub or collection created or changed here is read back as written, so the
 * walk proves the dialog sent what the navigation then draws.
 */
export const LOOK_HUB = { id: '01a0e2e0-0000-7000-8000-000000000081', type: 'HUB', parent_id: null, name: 'Agency', icon: 'briefcase', color_token: 'violet', order_key: 'a3', version: 1 };
export const LOOK_PRIVATE_HUB = { id: '01a0e2e0-0000-7000-8000-000000000082', type: 'HUB', parent_id: null, name: 'Family', icon: 'heart', color_token: 'magenta', order_key: 'a4', version: 1, private: true };
export const LOOK_COLLECTION = { id: '01a0e2e0-0000-7000-8000-000000000083', type: 'COLLECTION', parent_id: LOOK_HUB.id, name: 'Clients', icon: 'rocket', color_token: 'amber', order_key: 'a0', version: 1 };
export const PLAIN_COLLECTION = { id: '01a0e2e0-0000-7000-8000-000000000084', type: 'COLLECTION', parent_id: LOOK_HUB.id, name: 'Invoices', order_key: 'a1', version: 1 };
export const LOOK_ITEMS = [
  item('85', 'Send the offer to the bakery', { collection_id: LOOK_COLLECTION.id, due_at: `${inDays(-1)}T09:00:00Z` }),
  item('86', 'Pay the tiler', { collection_id: PLAIN_COLLECTION.id, due_at: `${inDays(3)}T09:00:00Z` }),
  { ...ITEMS[1] },
];

let looksShown = typeof process !== 'undefined' && process.env?.HUBTASK_FIXTURE_LOOKS === '1';
/** Adds the places of their own to the tree, for this process. */
export function showLooks() {
  looksShown = true;
}
/** The look records as written so far; reset by `forgetUnstubbed`. */
let looked = [];
const freshLooks = () => [LOOK_HUB, LOOK_PRIVATE_HUB, LOOK_COLLECTION, PLAIN_COLLECTION].map((each) => ({ ...each }));

/** The look records' reads and writes; `undefined` where the general stub answers. */
function lookRoute(route, request, url, path) {
  if (!looksShown) return undefined;
  if (looked.length === 0) looked = freshLooks();
  const method = request.method();
  if (path.endsWith('/api/v1/containers') && method === 'POST') {
    const body = request.postDataJSON();
    const made = { id: `01a0e2e0-0000-7000-8000-0000000000${87 + looked.length - 4}`, parent_id: null, order_key: `a${looked.length + 5}`, version: 1, ...body };
    looked.push(made);
    return route.fulfill({ status: 201, json: made });
  }
  if (path.endsWith('/api/v1/containers') && url.searchParams.get('type') === 'HUB') {
    return route.fulfill({ json: { ...PAGE, data: [HUB, ...looked.filter((c) => c.type === 'HUB')] } });
  }
  const parent = url.searchParams.get('parent_id');
  if (path.endsWith('/api/v1/containers') && looked.some((c) => c.id === parent)) {
    return route.fulfill({ json: { ...PAGE, data: looked.filter((c) => c.parent_id === parent) } });
  }
  const index = looked.findIndex((c) => path.endsWith(`/api/v1/containers/${c.id}`));
  if (index >= 0 && method === 'PATCH') {
    looked[index] = { ...looked[index], ...request.postDataJSON(), version: looked[index].version + 1 };
    return route.fulfill({ json: looked[index] });
  }
  if (index >= 0) return route.fulfill({ json: looked[index] });
  if (path.endsWith('/api/v1/search')) return route.fulfill({ json: { ...PAGE, data: LOOK_ITEMS } });
  if (path.endsWith('/api/v1/sync:snapshot')) {
    const records = [HUB, COLLECTION, ...looked].map((c) => JSON.stringify({ op: 'UPSERT', entity: 'container', entity_id: c.id, container_id: c.parent_id, payload: c }));
    return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${records.join('\n')}\n{"cursor":"c-e2e"}\n` });
  }
  return undefined;
}

export function stub(route) {
  const request = route.request();
  const url = new URL(request.url());
  const path = url.pathname;
  const looks = lookRoute(route, request, url, path);
  if (looks) return looks;
  const figured = figureRoute(route, request, url, path);
  if (figured) return figured;
  const celebrated = celebrationRoute(route, request, url, path);
  if (celebrated) return celebrated;
  // The stream, **accepted and empty**: the connection is what the mark in the bar reads,
  // so a walk that refused it would draw *Reconnecting…* on every screenshot of every
  // screen. It carries the server's own reconnect suggestion and no records; what a walk needs
  // from the stream is that it was opened.
  if (path.endsWith('/api/v1/stream')) {
    return route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'retry: 3600000\n\n' });
  }
  if (path.endsWith('/api/v1/sync:snapshot')) {
    // The grocery list travels whole - its labels and its entries too - so a walk offline reads
    // it from the local copy (UC-WRK-25 check 6).
    const records = [
      ...[HUB, COLLECTION, GROCERIES].map((c) => ({ op: 'UPSERT', entity: 'container', entity_id: c.id, container_id: c.parent_id, payload: c })),
      ...GROCERY_LABELS.map((label) => ({ op: 'UPSERT', entity: 'label', entity_id: label.id, container_id: GROCERIES.id, payload: { ...label, collection_id: GROCERIES.id } })),
      ...GROCERY_ITEMS.map((entry) => ({ op: 'UPSERT', entity: 'item', entity_id: entry.id, container_id: GROCERIES.id, payload: entry })),
    ].map((record) => JSON.stringify(record));
    return route.fulfill({ status: 200, contentType: 'application/x-ndjson', body: `${records.join('\n')}\n{"cursor":"c-e2e"}\n` });
  }
  if (path.endsWith('/api/v1/sync:pull')) return route.fulfill({ json: { changes: [], cursor: 'c-e2e', has_more: false, tombstone_window_days: 90 } });
  if (path.endsWith('/api/v1/meta/capabilities')) return route.fulfill({ json: MANIFEST });
  if (path.endsWith('/api/v1/accounts/me')) return route.fulfill({ json: ACCOUNT });
  // The reader's passkeys: none yet, and another can be added - at `localhost`, the host a walk opens
  // the bundle at when it needs a browser to make one (an address is no relying party).
  if (path.endsWith('/api/v1/accounts/me/passkeys')) return route.fulfill({ json: PASSKEYS });
  if (path.endsWith(`/api/v1/accounts/${ACCOUNT.id}`)) return route.fulfill({ json: ACCOUNT });
  if (path.endsWith(`/api/v1/accounts/${OTHER.id}`)) return route.fulfill({ json: OTHER });
  if (path.endsWith(`/api/v1/accounts/${INVITED.id}`)) return route.fulfill({ json: INVITED });
  // `include_archived` is honoured, because it is the whole of what the archive screen asks and a
  // stub that ignored it would prove the screen works by handing it rows the product hides.
  if (path.endsWith('/api/v1/containers')) {
    const archived = url.searchParams.get('include_archived') === 'true';
    const rows = url.searchParams.get('type') === 'HUB' ? [HUB] : [COLLECTION, ...(archived ? [ARCHIVED] : [])];
    return route.fulfill({ json: { ...PAGE, data: rows } });
  }
  if (path.endsWith(`/api/v1/containers/${COLLECTION.id}`)) return route.fulfill({ json: COLLECTION });
  // The list's figure (UC-WRK-25 check 7, UX-24): the server counts the open entries on the read; the
  // replica's snapshot carries none, as no change record does.
  if (path.endsWith(`/api/v1/containers/${GROCERIES.id}`)) {
    const open = GROCERY_ITEMS.filter((entry) => !entry.completion?.is_completed).length;
    return route.fulfill({ json: { ...GROCERIES, figure: { kind: 'OPEN', open, today: today() } } });
  }
  if (path.endsWith(`/api/v1/containers/${GROCERIES.id}/labels`)) return route.fulfill({ json: GROCERY_LABELS });
  // A device's queued writes, answered as applied (offline-sync.md §3.2): the walk offline sees
  // its marks go when the network is back.
  if (path.endsWith('/api/v1/sync:push') && request.method() === 'POST') {
    const sent = request.postDataJSON();
    pushed.push(sent);
    return route.fulfill({ json: { results: (sent.mutations ?? []).map((mutation) => ({ op_id: mutation.op_id, result: 'APPLIED' })) } });
  }
  if (path.endsWith(`/api/v1/containers/${ARCHIVED.id}`)) return route.fulfill({ json: ARCHIVED });
  if (path.endsWith(`/api/v1/containers/${ARCHIVED_HUB.id}`)) return route.fulfill({ json: ARCHIVED_HUB });
  if (path.endsWith(`/api/v1/containers/${UNDER_ARCHIVED.id}`)) return route.fulfill({ json: UNDER_ARCHIVED });
  if (path.endsWith(`/api/v1/containers/${HUB.id}`)) return route.fulfill({ json: HUB });
  if (path.endsWith(`/api/v1/containers/${HUB.id}/privacy`) && request.method() === 'PUT') {
    return route.fulfill({ json: { ...HUB, private: request.postDataJSON().private, version: HUB.version + 1 } });
  }
  if (path.endsWith('/api/v1/tenant') && request.method() === 'GET') return route.fulfill({ json: WORKSPACE });
  if (path.endsWith('/api/v1/private-hubs') && request.method() === 'GET') return route.fulfill({ json: PRIVATE_HUBS });
  // A hub's emergency accesses, and the reader's own notices: none, unless a walk routes them.
  if (/\/api\/v1\/private-hubs\/[^/]+\/accesses$/.test(path) && request.method() === 'GET') return route.fulfill({ json: [] });
  if (path.endsWith('/labels')) return route.fulfill({ json: LABELS });
  if (path.endsWith('/buckets')) return route.fulfill({ json: BUCKETS });
  if (path.endsWith('/api/v1/memberships')) {
    const scope = url.searchParams.get('scope_type');
    const rows = scope === 'COLLECTION' ? [{ id: 'm1', scope_type: 'COLLECTION', scope_id: COLLECTION.id, account_id: OTHER.id, role: 'MEMBER' }] : scope === 'HUB' ? [{ id: 'm2', scope_type: 'HUB', scope_id: HUB.id, account_id: ACCOUNT.id, role: 'OWNER' }] : scope === 'TENANT' ? [{ id: 'm0', scope_type: 'TENANT', account_id: ACCOUNT.id, role: 'OWNER' }, { id: 'm9', scope_type: 'TENANT', account_id: OTHER.id, role: 'MEMBER' }] : [];
    return route.fulfill({ json: { ...PAGE, data: rows, grantable_roles: OWNER_GRANTS } });
  }
  if (path.endsWith('/api/v1/items:query')) {
    const body = request.postDataJSON();
    if (body.group_by) {
      const groups = [...BUCKETS.map((b) => b.id), null].map((key) => { const data = ITEMS.filter((i) => (i.bucket_id ?? null) === key); return { key, count: data.length, data, page: { next_cursor: null, has_more: false } }; });
      return route.fulfill({ json: { data: [], groups, page: { next_cursor: null, has_more: false }, total: ITEMS.length } });
    }
    if (body.scope?.item_id) {
      // One level, or the whole subtree in one answer - the anchor is not in its own subtree.
      const below = (id) => (CHILDREN[id] ?? []).flatMap((child) => [child, ...(body.scope.include_descendants ? below(child.id) : [])]);
      const data = below(body.scope.item_id);
      return route.fulfill({ json: { data, groups: [], page: { next_cursor: null, has_more: false }, total: data.length } });
    }
    const level = body.scope?.container_id === GROCERIES.id ? GROCERY_ITEMS : ITEMS;
    return route.fulfill({ json: { data: level, groups: [], page: { next_cursor: null, has_more: false }, total: level.length } });
  }
  const one = ALL_ITEMS.find((each) => path.endsWith(`/api/v1/items/${each.id}`));
  if (one && request.method() === 'GET') return route.fulfill({ json: one });
  if (one && request.method() === 'PATCH') return route.fulfill({ json: { ...one, ...request.postDataJSON(), version: one.version + 1 } });
  if (path.match(/\/api\/v1\/items\/[^/]+:(complete|reopen)$/) && request.method() === 'POST') {
    const id = path.split('/').pop().split(':')[0];
    const target = ALL_ITEMS.find((each) => each.id === id);
    const done = path.endsWith(':complete');
    // A grocery entry keeps what happened to it, so the list re-read after a tick draws it under
    // *In the trolley*, as a server would.
    const at = GROCERY_ITEMS.findIndex((each) => each.id === id);
    const current = at >= 0 ? GROCERY_ITEMS[at] : target;
    const next = { ...current, completion: done ? { is_completed: true, completed_at: new Date().toISOString() } : { is_completed: false }, completion_count: (current.completion_count ?? 0) + (done && !current.completion?.is_completed ? 1 : 0), version: current.version + 1 };
    if (at >= 0) GROCERY_ITEMS[at] = next;
    return route.fulfill({ json: next });
  }
  const covered = ALL_ITEMS.find((each) => path.endsWith(`/api/v1/items/${each.id}/cover`));
  if (covered && request.method() === 'PUT') return route.fulfill({ json: { ...covered, cover: request.postDataJSON(), version: covered.version + 1 } });
  if (covered && request.method() === 'DELETE') return route.fulfill({ json: { ...covered, cover: null, version: covered.version + 1 } });
  if (path.match(/\/api\/v1\/items\/[^/]+\/(reminders|attachments|comments|activity)$/)) return route.fulfill({ json: { ...PAGE, data: [] } });
  if (path.match(/\/api\/v1\/items\/[^/]+\/recurrence$/)) return route.fulfill({ status: 404, json: { code: 'recurrence.not_found' } });
  if (/\/(views|templates|custom-fields|policies|feeds)$/.test(path)) return route.fulfill({ json: [] });
  // What the holds would keep of an erasure: the hub's hold, over the person's comments there.
  if (path.endsWith('/erasure-preview') && request.method() === 'GET') {
    return route.fulfill({ json: { mode: url.searchParams.get('mode') ?? 'ANONYMIZE', kept: PRIVACY_REQUESTS[4].kept } });
  }
  if (path.endsWith('/api/v1/legal-holds') && request.method() === 'GET') return route.fulfill({ json: LEGAL_HOLDS });
  if (path.endsWith('/api/v1/identity-provider-presets')) return route.fulfill({ json: PROVIDER_PRESETS });
  // Rule drafting: the ask, its job, the asker's own listing, the acceptance and the rule it made.
  if (path.endsWith('/api/v1/automation/rules:describe') && request.method() === 'POST') {
    return route.fulfill({ status: 202, json: { job_id: DRAFT_JOB, status: 'QUEUED', progress: null, result_url: null } });
  }
  if (path.endsWith(`/api/v1/jobs/${DRAFT_JOB}`)) {
    return route.fulfill({ json: { job_id: DRAFT_JOB, status: 'SUCCEEDED', outcome_code: 'ai.proposed', progress: null, result_url: null, error_code: null, created_at: new Date().toISOString(), finished_at: new Date().toISOString() } });
  }
  if (path.endsWith('/api/v1/suggestions') && url.searchParams.get('target_type') === 'ACCOUNT') {
    return route.fulfill({ json: { items: [ruleDraft()], has_more: false, next_cursor: null } });
  }
  if (path.endsWith(`/api/v1/suggestions/${RULE_DRAFT_ID}:accept`) && request.method() === 'POST') {
    draftSaved = true;
    return route.fulfill({ json: { ...ruleDraft('ACCEPTED'), created_id: DRAFTED_RULE_ID } });
  }
  if (path.endsWith(`/api/v1/suggestions/${RULE_DRAFT_ID}:dismiss`) && request.method() === 'POST') {
    return route.fulfill({ json: ruleDraft('DISMISSED') });
  }
  if (path.endsWith(`/api/v1/automation/rules/${DRAFTED_RULE_ID}`)) return route.fulfill({ json: DRAFTED_RULE });
  if (path.endsWith('/api/v1/automation/rules') && request.method() === 'GET') return route.fulfill({ json: { ...PAGE, data: draftSaved ? [DRAFTED_RULE] : [] } });
  if (path.endsWith('/api/v1/admin/tenants') && request.method() === 'GET') return route.fulfill({ json: WORKSPACES });
  if (path.endsWith('/api/v1/admin/ai-providers') && request.method() === 'GET') return route.fulfill({ json: OFFERED_AI });
  if (path.endsWith('/api/v1/ai-source') && request.method() === 'GET') return route.fulfill({ json: AI_SOURCE });
  if (path.endsWith('/api/v1/privacy/ai-objection') && request.method() === 'GET') return route.fulfill({ json: AI_OBJECTION });
  if (path.endsWith('/api/v1/privacy/consents') && request.method() === 'GET') return route.fulfill({ json: CONSENT_REGISTER });
  if (path.endsWith('/api/v1/admin/encryption') && request.method() === 'GET') return route.fulfill({ json: ENCRYPTION_STATUS });
  if (path.endsWith('/api/v1/admin/subscriptions') && request.method() === 'GET') return route.fulfill({ json: PLATFORM_SUBSCRIPTIONS });
  // The operator's hour at the installation level, granted: a walk of those screens looks at them,
  // not at the door (ADR-0070 §4).
  if (path.endsWith('/api/v1/auth/sessions:elevate')) {
    return route.fulfill({ json: { elevated_until: new Date(Date.now() + 3_600_000).toISOString() } });
  }
  if (path.endsWith('/api/v1/privacy/requests') && request.method() === 'GET') {
    return route.fulfill({ json: { data: PRIVACY_REQUESTS, page: { next_cursor: null, has_more: false } } });
  }
  // An extension answers the case it makes: extended once, the bound gone, both dates kept.
  const extended = PRIVACY_REQUESTS.find((each) => path.endsWith(`/api/v1/privacy/requests/${each.id}:extend`));
  if (extended && request.method() === 'POST') {
    const body = request.postDataJSON();
    const { extendable_until: _gone, ...rest } = extended;
    return route.fulfill({ json: { ...rest, due_at: `${body.due_on}T22:59:59Z`, original_due_at: extended.due_at, extension_reason: body.reason, informed_on: body.informed_on } });
  }
  return fallback(route, request, path);
}

/**
 * The reads the frame makes on every screen, and a record of everything else.
 *
 * **Shapes, because a wrong one is silent.** The API answers an array where the client maps over
 * one and an object where it reads a field; a page envelope in either place puts an object through
 * `.find`, or `undefined` where a field belongs, and the screen throws *while rendering*. That is
 * a failure a fast machine hides: the next navigation usually wins the race, and the slower CI
 * runner is where the screen gets far enough to try. Measured before this was written, the silent
 * catch-all was answering `/quotas`, `/meta/health` and `/integrations/calendar-feeds` — three
 * reads the frame makes on *every* screen — in a shape none of them has.
 *
 * **A record, because a guess nobody notices is the whole problem.** Anything this fixture was
 * never asked for is still answered — a walk that died here would say less than one that finishes
 * — but its path is kept, and `signedIn` hands the record to the test. `unstubbed()` beside
 * `failures` is the assertion: the same shape the walks already use for what the page threw.
 *
 * Exported, so that a walk with a stub of its own ends with this rather than inventing a second
 * catch-all. Every one of them had the same wrong shapes for the same reason.
 */
export function fallback(route, request, path) {
  const read = path.replace(/^.*\/api\/v1/, '');

  // Bringing a container back, which is what the archive is for. Answered with the container it
  // makes rather than with a page: a guess here would be a guess in the very walk whose subject
  // it is.
  if (read === `/containers/${ARCHIVED.id}:unarchive`) {
    const { archived_at: _put, ...back } = ARCHIVED;
    return route.fulfill({ json: { ...back, version: ARCHIVED.version + 1 } });
  }
  // Words find the entries whose title holds them; a search with no words - the overview's - finds
  // nothing, so the screens that ask one are drawn empty as before.
  if (read === '/search') {
    const words = (request.postDataJSON()?.q ?? '').trim().toLowerCase();
    const hits = words === '' ? [] : ALL_ITEMS.filter((each) => each.title.toLowerCase().includes(words));
    return route.fulfill({ json: { ...PAGE, data: hits } });
  }

  // The three writes the walks make, each answered with the entry it
  // makes. Two of them are the subject of the walk that makes them - the timeline's drag sets a
  // due date, the board's carry reorders - so a guess here is a walk that cannot claim to have
  // exercised what it is named after.
  const due = ALL_ITEMS.find((each) => read === `/items/${each.id}/due`);
  if (due) return route.fulfill({ json: { ...due, ...request.postDataJSON(), version: due.version + 1 } });
  const ranked = ALL_ITEMS.find((each) => read === `/items/${each.id}:reorder`);
  if (ranked) return route.fulfill({ json: { ...ranked, ...request.postDataJSON(), version: ranked.version + 1 } });
  // What an entry's menu writes (UC-WRK-28): each answered with what it makes, so a walk that
  // presses an item proves the request it sends rather than a guess.
  const lifecycle = read.match(/^\/items\/([^/:]+):(archive|unarchive|restore)$/);
  const living = lifecycle && ALL_ITEMS.find((each) => each.id === lifecycle[1]);
  if (living && request.method() === 'POST') {
    return route.fulfill({ json: { ...living, archived_at: lifecycle[2] === 'archive' ? '2026-10-21T09:00:00Z' : null, version: living.version + 1 } });
  }
  if (ALL_ITEMS.some((each) => read === `/items/${each.id}`) && request.method() === 'DELETE') return route.fulfill({ status: 204, body: '' });
  const moved = read.match(/^\/items\/([^/:]+):move$/);
  const carried = moved && ALL_ITEMS.find((each) => each.id === moved[1]);
  if (carried && request.method() === 'POST') return route.fulfill({ json: { item: { ...carried, version: carried.version + 1 }, dropped_references: [] } });
  const tagged = read.match(/^\/items\/([^/]+)\/labels\/([^/]+)$/);
  const labelled = tagged && ALL_ITEMS.find((each) => each.id === tagged[1]);
  if (labelled && (request.method() === 'PUT' || request.method() === 'DELETE')) {
    const others = (labelled.label_ids ?? []).filter((id) => id !== tagged[2]);
    return route.fulfill({ json: { label_ids: request.method() === 'PUT' ? [...others, tagged[2]] : others } });
  }
  const vocabulary = read.match(/^\/containers\/[^/]+\/labels\/([^/:]+)(:restore)?$/);
  const word = vocabulary && LABELS.find((each) => each.id === vocabulary[1]);
  if (word && request.method() === 'DELETE') return route.fulfill({ status: 204, body: '' });
  if (word && request.method() === 'POST' && vocabulary[2]) return route.fulfill({ json: { ...word, deleted_at: null, version: (word.version ?? 1) + 1 } });
  if (read === '/items:bulk' && request.method() === 'POST') {
    const operations = request.postDataJSON()?.operations ?? [];
    const results = operations.map((operation, index) => {
      const item = ALL_ITEMS.find((each) => each.id === operation.item_id);
      return { index, status: 200, item: item ? { ...item, version: item.version + 1 } : undefined };
    });
    return route.fulfill({ json: { applied: results.length, failed: 0, results } });
  }
  if (read === '/items' && request.method() === 'POST') {
    created = { ...ALL_ITEMS[0], id: CREATED_ID, ...request.postDataJSON(), version: 1 };
    return route.fulfill({ status: 201, json: created });
  }
  // The entry a walk just made, read back and trashed again - what a capture's *Undo* does.
  if (read === `/items/${CREATED_ID}` && request.method() === 'GET' && created) return route.fulfill({ json: created });
  if (read === `/items/${CREATED_ID}` && request.method() === 'DELETE') return route.fulfill({ status: 204, body: '' });
  // Several entries trashed at once, all or nothing: every operation applied.
  if (read === '/items:bulk' && request.method() === 'POST') {
    const operations = request.postDataJSON().operations ?? [];
    return route.fulfill({ json: { results: operations.map(() => ({ status: 204 })) } });
  }
  // A capture into the jumble, and its *Undo*: the entry, then the same entry dismissed.
  if (read === '/jumble/entries' && request.method() === 'POST') {
    const body = request.postDataJSON();
    return route.fulfill({ status: 201, json: { ...JUMBLED, raw_subject: body.raw_subject ?? null, channel: body.channel } });
  }
  if (read === `/jumble/entries/${JUMBLED.id}:dismiss` && request.method() === 'POST') {
    return route.fulfill({ json: { ...JUMBLED, status: 'DISMISSED' } });
  }

  if (ARRAYS.has(read)) return route.fulfill({ json: [] });
  if (read === '/meta/health') return route.fulfill({ json: { status: 'ok', degraded_features: [] } });
  if (read === '/jumble/entries') return route.fulfill({ json: { data: [], next_cursor: null } });
  if (read === '/jumble/destinations') return route.fulfill({ json: { data: DESTINATIONS, page: { next_cursor: null, has_more: false } } });

  unstubbed.push(`${request.method()} ${read}`);
  return route.fulfill({ json: PAGE });
}

/**
 * The reads that answer a bare array rather than a page envelope.
 *
 * One list, because the mistake is one mistake: every walk that wrote its own had the same three
 * wrong. A walk that visits the administration meets most of them without ever naming one.
 */
const ARRAYS = new Set([
  '/quotas', '/groups', '/retention-policies', '/oauth/clients', '/oauth/grants',
  '/auth/service-accounts', '/auth/tokens', '/auth/sessions', '/sync/devices',
  '/backup-targets', '/backup-schedules', '/backups', '/integrations/webhooks',
  '/integrations/calendar-feeds', '/private-hubs', '/accounts/me/emergency-notices',
  // The providers connected to the reader's own account, read by *Password and sign-in*.
  '/accounts/me/identities',
  // The provider surface, plural and read by the administration's sign-in screen. Both
  // answer a bare array, and a page envelope here is what "this.all.filter is not a function" looks
  // like from the outside.
  '/identity-providers', '/identity-provider-presets',
  // The installation's providers, read by its overview and its providers screen: a bare array too.
  '/admin/identity-providers',
  // The installation's backup targets, read by its workspaces screen for the export.
  '/admin/backup-targets',
]);

/** What the fixture was asked for and had no answer prepared for. Read through `signedIn`. */
let unstubbed = [];

/** The id every create answers with, and the entry the last create made. */
export const CREATED_ID = '01a0e2e0-0000-7000-8000-0000000000ff';
let created;

/** What a capture into the jumble makes. */
export const JUMBLED = { id: '01a0e2e0-0000-7000-8000-0000000007b1', channel: 'QUICK_CAPTURE', raw_subject: null, raw_body: null, attachments: [], status: 'NEW', received_at: '2026-10-21T08:00:00Z' };

/** Starts a fresh record. `signedIn` does it; a walk with its own context calls it itself. */
export function forgetUnstubbed() {
  unstubbed = [];
  created = undefined;
  pushed.length = 0;
  looked = [];
  draftSaved = false;
}

/** Every push a device sent, in order: what a walk offline reads back once the network returns. */
export const pushed = [];

/** Whether a walk saved the rule draft: the workspace's rules hold it from then on. */
let draftSaved = false;

/** What was asked for and guessed at, without repeats. */
export function unstubbedSoFar() {
  return [...new Set(unstubbed)];
}

/**
 * A signed-in tab at a width, with the failures the bundle throws collected. A walk about days
 * passes `timezoneId`, so it reads the same on every machine; a walk about fingers passes
 * `hasTouch`, so the page meets a touch screen. Nothing else is taken from `options`.
 */
export async function signedIn(browser, width, height = 800, { timezoneId, hasTouch } = {}) {
  const context = await browser.newContext({
    viewport: { width, height },
    ...(timezoneId ? { timezoneId } : {}),
    ...(hasTouch ? { hasTouch: true } : {}),
  });
  await context.route('**/api/v1/**', stub);
  await context.addInitScript(() => { sessionStorage.setItem('hubtask.bearer', 'e2e-bearer'); sessionStorage.setItem('hubtask.refresh', 'e2e-refresh'); });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  forgetUnstubbed();
  return { context, page, failures, unstubbed: unstubbedSoFar };
}
