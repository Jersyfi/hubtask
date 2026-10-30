// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's configuration items as the *Assets* screen needs them (M6, UC-AST-01 … UC-AST-13): the
 * paths it reads, the inventory grouped by class, a typed attribute read from what a person typed,
 * the relations both ways in their own words, the impact by depth, the protection need beside the
 * one inherited, duplicates side by side, and a file's format and columns. Pure, so the shapes
 * have tests without a browser.
 */

import type {
  CiAlert, CiAsset, CiAssetRegister, CiAttribute, CiCampaign, CiCampaignDetail, CiCampaignStart, CiClass, CiConnector, CiConnectorDefine,
  CiDecision, CiFileImport, CiGroup, CiGroupDefine, CiOwnProtection, CiProtection, CiReached, CiRelation, CiRelationType, ConfigurationItem,
  EntryCis, EntryOnCi, ServiceMap, SoftwareAliasDefine, SoftwareProductList,
} from '@hubtask/sync-engine';

export const ROOT_CLASS = 'ci';

export const classesPath = '/cmdb/classes';
export const relationTypesPath = '/cmdb/relation-types';
export const sourcesPath = '/cmdb/sources';

export function cisPath(hubId: string, options: { class?: string; after?: string; includeMerged?: boolean } = {}): string {
  const query = new URLSearchParams();
  if (options.class) query.set('class', options.class);
  if (options.after) query.set('after', options.after);
  if (options.includeMerged) query.set('include_merged', 'true');
  const tail = query.toString();
  return `/hubs/${hubId}/cis${tail ? `?${tail}` : ''}`;
}
export const ciPath = (ciId: string) => `/cis/${ciId}`;
export const impactPath = (ciId: string, direction: 'DOWNSTREAM' | 'UPSTREAM') => `/cis/${ciId}/impact?direction=${direction}`;
export const runsPath = (hubId: string) => `/hubs/${hubId}/ci-runs`;
export const runPath = (runId: string) => `/ci-runs/${runId}`;
export const holdsPath = (hubId: string) => `/hubs/${hubId}/ci-holds`;
export const duplicatesPath = (hubId: string) => `/hubs/${hubId}/ci-duplicates`;
export const healthPath = (hubId: string) => `/hubs/${hubId}/cmdb-health`;

/** The sections of a hub's *Assets* screen, in their order; the first is the address's default. */
export const ASSET_SECTIONS = ['inventory', 'runs', 'duplicates', 'import', 'connectors', 'alerts', 'health', 'campaigns', 'groups', 'register', 'software'] as const;
export type AssetSection = (typeof ASSET_SECTIONS)[number];
export const sectionOf = (name: string | undefined): AssetSection =>
  (ASSET_SECTIONS as readonly string[]).includes(name ?? '') ? (name as AssetSection) : 'inventory';

/** Looks a word up and says whether the catalogue has it; the screen passes its `t`. */
export type Words = (code: string) => string | undefined;

/**
 * A class's word: an own class's name, a shipped class's word from the catalogue, else its key -
 * a shipped class carries no name (the contract's `CiClass`).
 */
export function classWord(key: string, classes: readonly CiClass[], words: Words): string {
  const own = classes.find((each) => each.key === key)?.name;
  return own || words(`app.assets.class.${key}`) || key;
}

/** A relation type's word read from one end: from its source the name, from its target the reverse. */
export function relationWord(type: CiRelationType | undefined, key: string, outgoing: boolean, words: Words): string {
  const own = outgoing ? type?.name : type?.reverse_name;
  return own || words(`app.assets.relation.${key}.${outgoing ? 'out' : 'in'}`) || key;
}

/** The classes a CI may be made of: not abstract, not archived, not the root. */
export const instantiable = (classes: readonly CiClass[]): CiClass[] =>
  classes.filter((each) => !each.abstract && !each.archived && each.key !== ROOT_CLASS);

/** The classes beneath a CI's own, which it may be refined to (`cmdb.class_not_descendant`). */
export const refinements = (classes: readonly CiClass[], key: string): CiClass[] =>
  instantiable(classes).filter((each) => each.key !== key && each.lineage.includes(key));

export interface ClassGroup {
  readonly class: string;
  readonly items: readonly ConfigurationItem[];
}

/** The inventory by class, in the order the classes are listed; a class unknown to the list last. */
export function inventoryByClass(cis: readonly ConfigurationItem[], classes: readonly CiClass[]): ClassGroup[] {
  const order = new Map(classes.map((each, index) => [each.key, index]));
  const groups = new Map<string, ConfigurationItem[]>();
  for (const ci of cis) groups.set(ci.class, [...(groups.get(ci.class) ?? []), ci]);
  return [...groups.entries()]
    .sort(([a], [b]) => (order.get(a) ?? Number.MAX_SAFE_INTEGER) - (order.get(b) ?? Number.MAX_SAFE_INTEGER) || a.localeCompare(b))
    .map(([key, items]) => ({ class: key, items }));
}

/** What a typed field's text becomes, or why it does not: an empty field clears the value. */
export type Parsed = { readonly ok: true; readonly value: unknown } | { readonly ok: false };

export function parseAttribute(attribute: CiAttribute, text: string): Parsed {
  const value = text.trim();
  if (value === '') return { ok: true, value: null };
  switch (attribute.kind) {
    case 'NUMBER': {
      const number = Number(value.replace(',', '.'));
      return Number.isFinite(number) ? { ok: true, value: number } : { ok: false };
    }
    case 'BOOLEAN':
      return value === 'true' || value === 'false' ? { ok: true, value: value === 'true' } : { ok: false };
    case 'DATE':
      return /^\d{4}-\d{2}-\d{2}$/u.test(value) ? { ok: true, value } : { ok: false };
    case 'CHOICE':
      return (attribute.options ?? []).includes(value) ? { ok: true, value } : { ok: false };
    default:
      return { ok: true, value };
  }
}

/** A value as the field shows it. */
export function attributeText(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

/**
 * The attributes a form changed, with `null` for one cleared, or the keys that do not parse. Only
 * what changed is sent: the reconciliation keeps a value a higher-ranked source wrote, and a
 * field left alone must not claim it for `manual`.
 */
export function changedAttributes(
  attributes: readonly CiAttribute[],
  before: Record<string, unknown>,
  drafts: Record<string, string>,
): { readonly changes: Record<string, unknown>; readonly invalid: readonly string[] } {
  const changes: Record<string, unknown> = {};
  const invalid: string[] = [];
  for (const attribute of attributes) {
    const text = drafts[attribute.key];
    if (text === undefined || text === attributeText(before[attribute.key])) continue;
    const parsed = parseAttribute(attribute, text);
    if (!parsed.ok) invalid.push(attribute.key);
    else if (!(parsed.value === null && before[attribute.key] === undefined)) changes[attribute.key] = parsed.value;
  }
  return { changes, invalid };
}

/** The required attributes the form leaves empty, so the screen says so before asking. */
export const missingRequired = (attributes: readonly CiAttribute[], values: Record<string, string>): string[] =>
  attributes.filter((each) => each.need === 'REQUIRED' && !(values[each.key] ?? '').trim()).map((each) => each.key);

export interface RelationRow {
  readonly id: string;
  readonly type: string;
  readonly outgoing: boolean;
  /** The CI at the other end. */
  readonly other: string;
  readonly source: string;
  readonly lastSeen: string;
}

/** A CI's relations both ways, outgoing first, each named from this CI's end (UC-AST-01 check 5). */
export function relationRows(ciId: string, relations: readonly CiRelation[]): RelationRow[] {
  const row = (relation: CiRelation, outgoing: boolean): RelationRow => ({
    id: relation.id,
    type: relation.type,
    outgoing,
    other: outgoing ? relation.target_ci_id : relation.source_ci_id,
    source: relation.data_source,
    lastSeen: relation.last_seen,
  });
  return [
    ...relations.filter((each) => each.source_ci_id === ciId).map((each) => row(each, true)),
    ...relations.filter((each) => each.source_ci_id !== ciId && each.target_ci_id === ciId).map((each) => row(each, false)),
  ];
}

/** Whether a type may join a CI of one class to one of another; no pair joins any (`Allows`). */
export function typeAllows(type: CiRelationType, from: CiClass | undefined, to: CiClass | undefined): boolean {
  if (type.archived) return false;
  if (type.pairs.length === 0) return true;
  if (!from || !to) return false;
  return type.pairs.some((pair) => from.lineage.includes(pair.from) && to.lineage.includes(pair.to));
}

/** The impact as a list grouped by depth, the nearest first (UC-AST-05 check 5). */
export function impactByDepth(reached: readonly CiReached[]): { readonly depth: number; readonly items: readonly CiReached[] }[] {
  const depths = [...new Set(reached.map((each) => each.depth))].sort((a, b) => a - b);
  return depths.map((depth) => ({ depth, items: reached.filter((each) => each.depth === depth) }));
}

export const PROTECTION_LEVELS = ['NORMAL', 'HIGH', 'VERY_HIGH'] as const;
export type Level = (typeof PROTECTION_LEVELS)[number];
export const DIMENSIONS = ['c', 'i', 'a'] as const;
export type Dimension = (typeof DIMENSIONS)[number];

export interface ProtectionRow {
  readonly dimension: Dimension;
  readonly own: Level | null;
  readonly effective: Level | null;
  readonly fixed: boolean;
  /** The need was raised by what suffers when this CI fails. */
  readonly raised: boolean;
}

const rank = (level: Level | null | undefined) => (level ? PROTECTION_LEVELS.indexOf(level) : -1);

/** The own need beside the effective one, per dimension (UC-AST-05 check 5). */
export function protectionRows(own: CiOwnProtection | undefined, effective: CiProtection | undefined): ProtectionRow[] {
  return DIMENSIONS.map((dimension) => {
    const mine = (own?.[dimension] ?? null) as Level | null;
    const inherited = (effective?.[dimension] ?? mine) as Level | null;
    return {
      dimension,
      own: mine,
      effective: inherited,
      fixed: own?.fixed?.includes(dimension) ?? false,
      raised: rank(inherited) > rank(mine),
    };
  });
}

/** Two or more CIs side by side: every attribute any of them holds, and whether they differ. */
export function sideBySide(cis: readonly ConfigurationItem[]): { readonly key: string; readonly values: readonly string[]; readonly differ: boolean }[] {
  const keys = [...new Set(cis.flatMap((ci) => Object.keys(ci.attributes)))].sort();
  return [
    { key: 'name', values: cis.map((ci) => ci.name) },
    { key: 'class', values: cis.map((ci) => ci.class) },
    ...keys.map((key) => ({ key, values: cis.map((ci) => attributeText(ci.attributes[key])) })),
  ].map((row) => ({ ...row, differ: new Set(row.values).size > 1 }));
}

/** The tone a decision is drawn in; a refusal and a hold ask for a person. */
export function outcomeTone(outcome: CiDecision['outcome']): 'success' | 'info' | 'warning' | 'danger' | 'neutral' {
  switch (outcome) {
    case 'CREATE':
      return 'success';
    case 'UPDATE':
      return 'info';
    case 'HOLD':
    case 'DUPLICATE':
      return 'warning';
    case 'REFUSED':
      return 'danger';
    default:
      return 'neutral';
  }
}

/** A share from 0 to 1 as whole percent. */
export const percent = (share: number) => Math.round(Math.min(1, Math.max(0, share)) * 100);

/** The format a file is read in, by its name and what it holds. */
export function fileFormat(name: string, content: string): CiFileImport['format'] {
  if (/\.csv$/iu.test(name)) return 'CSV';
  const head = content.slice(0, 4096);
  if (/"bomFormat"\s*:\s*"CycloneDX"/u.test(head)) return 'CYCLONEDX';
  if (/"spdxVersion"\s*:/u.test(head) || /^SPDXVersion:/mu.test(head)) return 'SPDX';
  return /^\s*[[{]/u.test(content) ? 'JSON' : 'CSV';
}

/** A CSV table's or a JSON array's column names, for the mapping's pickers. */
export function fileColumns(format: CiFileImport['format'], content: string): string[] {
  if (format === 'CSV') {
    const first = content.split(/\r?\n/u, 1)[0] ?? '';
    const separator = (first.match(/;/gu)?.length ?? 0) > (first.match(/,/gu)?.length ?? 0) ? ';' : ',';
    return splitCsvLine(first, separator).map((each) => each.trim()).filter(Boolean);
  }
  if (format === 'JSON') {
    try {
      const parsed: unknown = JSON.parse(content);
      const rows = Array.isArray(parsed) ? parsed : [];
      return [...new Set(rows.slice(0, 50).flatMap((row) => (row && typeof row === 'object' ? Object.keys(row) : [])))];
    } catch {
      return [];
    }
  }
  return [];
}

function splitCsvLine(line: string, separator: string): string[] {
  const out: string[] = [];
  let field = '';
  let quoted = false;
  for (let i = 0; i < line.length; i++) {
    const char = line[i];
    if (quoted) {
      if (char === '"' && line[i + 1] === '"') { field += '"'; i++; }
      else if (char === '"') quoted = false;
      else field += char;
    } else if (char === '"') quoted = true;
    else if (char === separator) { out.push(field); field = ''; }
    else field += char;
  }
  out.push(field);
  return out;
}

/**
 * A mapping guessed from the columns: a column named as an attribute of the class maps to it, and
 * `name` / `hostname` and `id` / `key` are the name and the source key. A person changes it.
 */
export function guessMapping(columns: readonly string[], attributes: readonly CiAttribute[]): { name?: string; nativeKey?: string; columns: Record<string, string> } {
  const lower = (each: string) => each.toLowerCase().replace(/[^a-z0-9]+/gu, '_');
  const name = columns.find((each) => ['name', 'hostname', 'title'].includes(lower(each)));
  const nativeKey = columns.find((each) => ['id', 'key', 'asset_id', 'native_key'].includes(lower(each)));
  const mapped: Record<string, string> = {};
  for (const column of columns) {
    if (column === name || column === nativeKey) continue;
    const attribute = attributes.find((each) => each.key === lower(column));
    if (attribute) mapped[column] = attribute.key;
  }
  return { ...(name ? { name } : {}), ...(nativeKey ? { nativeKey } : {}), columns: mapped };
}

// --- The administration's definitions (UC-AST-02 check 6, UC-AST-06 check 5) -------------------

export interface AttributeDraft {
  key: string;
  kind: CiAttribute['kind'];
  need: NonNullable<CiAttribute['need']>;
  identifier: boolean;
  /** A choice's options, one per line. */
  options: string;
}

export interface ClassDraft {
  key: string;
  parent: string;
  name: string;
  abstract: boolean;
  attributes: AttributeDraft[];
  /** One identifier rule per line, its attributes joined by `+`. */
  rules: string;
  required: { types: string[]; outgoing: boolean }[];
  staleness: string;
  archived: boolean;
}

const lines = (text: string) => text.split(/\r?\n/u).map((each) => each.trim()).filter(Boolean);

/** A class as its editor holds it: only what the class itself declares, not what it inherits. */
export function classDraft(of?: CiClass): ClassDraft {
  return {
    key: of?.key ?? '',
    parent: of?.parent ?? ROOT_CLASS,
    name: of?.name ?? '',
    abstract: of?.abstract ?? false,
    attributes: (of?.attributes ?? []).map((each) => ({
      key: each.key, kind: each.kind, need: each.need ?? 'OPTIONAL', identifier: each.identifier ?? false, options: (each.options ?? []).join('\n'),
    })),
    rules: (of?.identifier_rules ?? []).filter((rule) => !rule.class || rule.class === of?.key).map((rule) => rule.attributes.join('+')).join('\n'),
    required: (of?.required_relations ?? []).map((each) => ({ types: [...each.types], outgoing: each.outgoing })),
    staleness: of && of.staleness_days ? String(of.staleness_days) : '',
    archived: of?.archived ?? false,
  };
}

/** The body `defineCiClass` takes; a staleness left empty inherits (0). */
export function classDefine(draft: ClassDraft) {
  return {
    parent: draft.parent,
    name: draft.name.trim(),
    abstract: draft.abstract,
    attributes: draft.attributes.filter((each) => each.key.trim()).map((each) => ({
      key: each.key.trim(),
      kind: each.kind,
      need: each.need,
      identifier: each.identifier,
      ...(each.kind === 'CHOICE' ? { options: lines(each.options) } : {}),
    })),
    identifier_rules: lines(draft.rules).map((line) => ({ attributes: line.split('+').map((each) => each.trim()).filter(Boolean) })),
    required_relations: draft.required.filter((each) => each.types.length > 0),
    staleness_days: Math.max(0, Number.parseInt(draft.staleness, 10) || 0),
    archived: draft.archived,
  };
}

/** A key as a person may type one: small letters, digits and underscores, starting with a letter. */
export const keyIsWellFormed = (key: string) => /^[a-z][a-z0-9_]{0,62}$/u.test(key);

/** The text a mention or a card of a CI is written as in notes, a page or a board's words. */
export const ciReferenceText = (ciId: string, strength: 'MENTION' | 'CARD' = 'MENTION') =>
  `${strength === 'CARD' ? '!' : ''}[](hubtask:ci/${ciId})`;

export const backlinksPath = (ciId: string) => `/cis/${ciId}/backlinks`;

/**
 * The CIs a writer is offered for a mention: not merged away, their names containing every word
 * typed, whatever the case, those whose name starts with the words first; all of them, by name,
 * while nothing is typed.
 */
export function matchingCis(cis: readonly ConfigurationItem[], term: string, limit: number): ConfigurationItem[] {
  const words = term.toLocaleLowerCase().split(/\s+/u).filter(Boolean);
  const wanted = term.trim().toLocaleLowerCase();
  return cis
    .filter((ci) => !ci.merged_into && words.every((word) => ci.name.toLocaleLowerCase().includes(word)))
    .sort((a, b) => {
      const x = a.name.toLocaleLowerCase().startsWith(wanted) ? 0 : 1;
      const y = b.name.toLocaleLowerCase().startsWith(wanted) ? 0 : 1;
      return x - y || a.name.localeCompare(b.name);
    })
    .slice(0, limit);
}

export const connectorsPath = (hubId: string) => `/hubs/${hubId}/cmdb-connectors`;
export const connectorPath = (hubId: string, key: string) => `${connectorsPath(hubId)}/${encodeURIComponent(key)}`;
export const connectorCredentialPath = (connectorId: string) => `/cmdb-connectors/${connectorId}/credential`;
export const connectorRunPath = (connectorId: string) => `/cmdb-connectors/${connectorId}:run`;
export function alertsPath(hubId: string, status?: CiAlert['status']): string {
  return `/hubs/${hubId}/ci-alerts${status ? `?status=${status}` : ''}`;
}

export type ConnectorKind = CiConnector['kind'];

/**
 * What each kind needs, as the domain's `ConnectorSpec` states it: whether it is posted to rather
 * than pulled, and the settings it cannot run without. The server refuses a missing one by its
 * code either way; the form asks for them so nobody types a key by hand. A switch is a setting of two
 * values, the empty one sent as none.
 */
export const CONNECTOR_KINDS: readonly {
  readonly kind: ConnectorKind;
  readonly push: boolean;
  readonly inventory?: boolean;
  readonly settings: readonly string[];
  readonly switches?: readonly ConnectorSwitch[];
}[] = [
  { kind: 'GLPI', push: false, settings: ['client_id'] },
  { kind: 'NETBOX', push: false, settings: [], switches: [{ key: 'dialect', on: 'nautobot', off: '' }] },
  { kind: 'INTUNE', push: false, settings: ['tenant', 'client_id', 'authority'], switches: [{ key: 'detected_apps', on: '', off: 'off' }] },
  { kind: 'ENTRA', push: false, settings: ['tenant', 'client_id', 'authority'] },
  { kind: 'HETZNER', push: false, settings: [] },
  { kind: 'OPENSTACK', push: false, settings: ['application_credential_id'] },
  { kind: 'ALERTMANAGER', push: true, settings: [] },
  { kind: 'ZABBIX', push: true, settings: [] },
  { kind: 'CHECKMK', push: true, settings: [] },
  { kind: 'GLPI_AGENT', push: true, inventory: true, settings: [] },
];
export interface ConnectorSwitch {
  readonly key: string;
  readonly on: string;
  readonly off: string;
}
/** Whether a draft's switch is on: a value other than the two counts as off. */
export const switchOn = (draft: ConnectorDraft, each: ConnectorSwitch) => (draft.settings[each.key] ?? '') === each.on;
export const kindSpec = (kind: ConnectorKind) => CONNECTOR_KINDS.find((each) => each.kind === kind) ?? { kind, push: false, settings: [] };

/** The periods offered, in minutes; 0 runs only when asked. The server takes any from 15 to a week. */
export const CONNECTOR_PERIODS = [0, 15, 60, 360, 1440, 10080] as const;

export interface ConnectorDraft {
  key: string;
  kind: ConnectorKind;
  source: string;
  baseUrl: string;
  settings: Record<string, string>;
  /** One host per line. */
  hosts: string;
  every: number;
  enabled: boolean;
  collection: string;
}

export function connectorDraft(of?: CiConnector): ConnectorDraft {
  return {
    key: of?.key ?? '',
    kind: of?.kind ?? 'NETBOX',
    source: of?.source ?? '',
    baseUrl: of?.base_url ?? '',
    settings: { ...(of?.settings ?? {}) },
    hosts: (of?.allowed_hosts ?? []).join('\n'),
    every: of?.every_minutes ?? 0,
    enabled: of?.enabled ?? true,
    collection: of?.collection_id ?? '',
  };
}

/**
 * The body `defineCiConnector` takes: only the kind's own settings, no address or period for an
 * intake and a collection only for an alert intake - each of which the server would refuse.
 */
export function connectorDefine(draft: ConnectorDraft): CiConnectorDefine {
  const spec = kindSpec(draft.kind);
  const settings = Object.fromEntries([
    ...spec.settings.map((key) => [key, (draft.settings[key] ?? '').trim()]),
    ...(spec.switches ?? []).map((each) => [each.key, switchOn(draft, each) ? each.on : each.off]),
  ].filter(([, value]) => value));
  return {
    kind: draft.kind,
    source: draft.source,
    enabled: draft.enabled,
    allowed_hosts: lines(draft.hosts),
    ...(Object.keys(settings).length ? { settings } : {}),
    ...(spec.push
      ? { every_minutes: 0, ...(draft.collection && !spec.inventory ? { collection_id: draft.collection } : {}) }
      : { base_url: draft.baseUrl.trim(), every_minutes: draft.every }),
  };
}

/** What the form still lacks, as message codes in the order they are asked; empty when it may be saved. */
export function connectorMissing(draft: ConnectorDraft, isNew: boolean): string[] {
  const spec = kindSpec(draft.kind);
  const out: string[] = [];
  if (isNew && !/^[a-z][a-z0-9_]{0,49}$/u.test(draft.key)) out.push('cmdb.connector_key_malformed');
  if (!draft.source) out.push('app.assets.connector_source_missing');
  if (!spec.push && !/^https:\/\/[^\s/]+/u.test(draft.baseUrl.trim())) out.push('cmdb.connector_base_url_invalid');
  for (const key of spec.settings) if (!(draft.settings[key] ?? '').trim()) out.push('cmdb.connector_setting_required');
  return out;
}

/**
 * Where a program posts to an intake - a monitoring program its alerts, a GLPI Agent its inventory
 * (the agent's server address): the API's origin is the interface's (ADR-0028).
 */
export function intakeAddress(origin: string, tenantId: string, connector: Pick<CiConnector, 'id' | 'inventory'>): string {
  const door = connector.inventory ? 'inventory-intake' : 'alert-intake';
  return `${origin.replace(/\/+$/u, '')}/api/v1/cmdb/${door}/${tenantId}/${connector.id}`;
}

/** How a connector's last run ended, as a badge's tone; never run is neutral. */
export function connectorTone(outcome: CiConnector['last_outcome']): 'success' | 'warning' | 'danger' | 'neutral' {
  if (outcome === 'OK') return 'success';
  if (outcome === 'EMPTY') return 'warning';
  if (outcome === 'FAILED') return 'danger';
  return 'neutral';
}

/** The key of a pull's alert about its own runs (the domain's `RunAlertKey`); its title is a message code. */
export const RUN_ALERT_KEY = 'hubtask.connector_run';

/** A firing alert is a problem; a resolved one is history. */
export const alertTone = (alert: CiAlert): 'danger' | 'neutral' => (alert.status === 'FIRING' ? 'danger' : 'neutral');

// --- The inventory's assurance (UC-AST-09 … UC-AST-13) ------------------------------------------

export const entryCisPath = (itemId: string) => `/items/${itemId}/cis`;
export const entryCiPath = (itemId: string, ciId: string) => `${entryCisPath(itemId)}/${ciId}`;
export const campaignsPath = (hubId: string) => `/hubs/${hubId}/ci-campaigns`;
export const campaignPath = (campaignId: string) => `/ci-campaigns/${campaignId}`;
export const attestPath = (campaignId: string, ciId: string) => `${campaignPath(campaignId)}/cis/${ciId}`;
export const campaignClosePath = (campaignId: string) => `${campaignPath(campaignId)}:close`;
export const groupsPath = (hubId: string) => `/hubs/${hubId}/ci-groups`;
export const groupPath = (hubId: string, key: string) => `${groupsPath(hubId)}/${encodeURIComponent(key)}`;
export const groupMembersPath = (groupId: string) => `/ci-groups/${groupId}/members`;
export const serviceMapPath = (hubId: string, serviceId?: string) =>
  `/hubs/${hubId}/service-map${serviceId ? `?service_id=${serviceId}` : ''}`;
export const assetsPath = (hubId: string, state?: AssetState) => `/hubs/${hubId}/assets${state ? `?state=${state}` : ''}`;
export const ciAssetPath = (ciId: string) => `/cis/${ciId}/asset`;
export const ciAssetMovePath = (ciId: string) => `${ciAssetPath(ciId)}:move`;
export const softwareAliasesPath = '/software-aliases';
export const softwareAliasPath = (aliasId: string) => `${softwareAliasesPath}/${aliasId}`;
export const softwarePath = (hubId: string) => `/hubs/${hubId}/software`;

/** An entry's link to another, by its collection's handle and number - `OPS-12`. */
export const entryKey = (entry: Pick<EntryOnCi, 'handle' | 'number'>) => `${entry.handle.toUpperCase()}-${entry.number}`;

/** The services the affected CIs reach, by name, an offering before a service of the same name. */
export const reachedServices = (answer: EntryCis): EntryCis['services'] =>
  [...answer.services].sort((a, b) => a.name.localeCompare(b.name) || a.class.localeCompare(b.class));

/** A collision on the CI itself first, then by the other entry's start. */
export const collisionsInOrder = (answer: EntryCis): EntryCis['collisions'] =>
  [...answer.collisions].sort((a, b) => Number(b.direct) - Number(a.direct) || (a.start_at ?? '').localeCompare(b.start_at ?? ''));

// Campaigns (UC-AST-10).

export interface CampaignDraft {
  name: string;
  classes: string[];
  /** Only CIs whose review falls due by this day; empty asks every CI of the classes. */
  dueBy: string;
  dueOn: string;
  months: string;
}

export const campaignDraft = (): CampaignDraft => ({ name: '', classes: [], dueBy: '', dueOn: '', months: '12' });

/** The body `startCiCampaign` takes; a period left empty is the server's twelve months. */
export function campaignStart(draft: CampaignDraft): CiCampaignStart {
  const months = Number.parseInt(draft.months, 10);
  return {
    name: draft.name.trim(),
    due_on: draft.dueOn,
    ...(draft.classes.length > 0 ? { classes: [...draft.classes] } : {}),
    ...(draft.dueBy ? { due_by: draft.dueBy } : {}),
    ...(Number.isFinite(months) ? { review_months: months } : {}),
  };
}

/** What the form still lacks, as message codes; the day it is due must lie ahead of `today`. */
export function campaignMissing(draft: CampaignDraft, today: string): string[] {
  const out: string[] = [];
  if (!draft.name.trim()) out.push('cmdb.campaign_name_invalid');
  if (!/^\d{4}-\d{2}-\d{2}$/u.test(draft.dueOn) || draft.dueOn <= today) out.push('cmdb.campaign_due_past');
  const months = Number(draft.months);
  if (draft.months.trim() && !(Number.isInteger(months) && months >= 1 && months <= 60)) out.push('cmdb.campaign_period_invalid');
  return out;
}

type Attestation = CiCampaignDetail['attestations'][number];

/**
 * A campaign's questions as its screen lists them: those the reader is asked and has not answered
 * first, then the others pending, then the answered, each by name.
 */
export function attestationsInOrder(detail: Pick<CiCampaignDetail, 'attestations'>, me: string | undefined): Attestation[] {
  const weight = (each: Attestation) => (each.verdict !== 'PENDING' ? 2 : me && each.owner_id === me ? 0 : 1);
  return [...detail.attestations].sort((a, b) => weight(a) - weight(b) || a.name.localeCompare(b.name));
}

/** Whether the reader may answer a question: the person asked with READ, anybody with WRITE_ITEMS. */
export const mayAttest = (campaign: Pick<CiCampaign, 'closed_at'>, each: Attestation, me: string | undefined, mayWrite: boolean) =>
  !campaign.closed_at && (mayWrite || (!!me && each.owner_id === me));

export function verdictTone(verdict: Attestation['verdict']): 'success' | 'warning' | 'neutral' {
  if (verdict === 'CONFIRMED') return 'success';
  if (verdict === 'DISPUTED') return 'warning';
  return 'neutral';
}

// Groups and the service map (UC-AST-11).

export const CI_STAGES = ['PLANNED', 'ORDERED', 'IN_STOCK', 'IN_USE', 'MAINTENANCE', 'RETIRED', 'DISPOSED'] as const;
type CiStage = (typeof CI_STAGES)[number];

export interface GroupDraft {
  key: string;
  name: string;
  classes: string[];
  stages: CiStage[];
  nameHas: string;
  /** One condition per line, `attribute = value`. */
  conditions: string;
  service: string;
}

export function groupDraft(of?: CiGroup): GroupDraft {
  return {
    key: of?.key ?? '',
    name: of?.name ?? '',
    classes: [...(of?.filter.classes ?? [])],
    stages: [...(of?.filter.stages ?? [])],
    nameHas: of?.filter.name_has ?? '',
    conditions: Object.entries(of?.filter.attributes ?? {}).map(([key, value]) => `${key} = ${value}`).join('\n'),
    service: of?.service_id ?? '',
  };
}

/** The attribute conditions a draft's lines hold; a line without `=` is not one. */
export function groupConditions(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of lines(text)) {
    const at = line.indexOf('=');
    if (at <= 0) continue;
    const key = line.slice(0, at).trim();
    if (key) out[key] = line.slice(at + 1).trim();
  }
  return out;
}

/** The body `defineCiGroup` takes: only the conditions given, the service cleared with `null`. */
export function groupDefine(draft: GroupDraft): CiGroupDefine {
  const attributes = groupConditions(draft.conditions);
  const nameHas = draft.nameHas.trim();
  return {
    name: draft.name.trim(),
    filter: {
      ...(draft.classes.length > 0 ? { classes: [...draft.classes] } : {}),
      ...(draft.stages.length > 0 ? { stages: [...draft.stages] } : {}),
      ...(nameHas ? { name_has: nameHas } : {}),
      ...(Object.keys(attributes).length > 0 ? { attributes } : {}),
    },
    service_id: draft.service || null,
  };
}

export function groupMissing(draft: GroupDraft, isNew: boolean): string[] {
  const out: string[] = [];
  if (isNew && !/^[a-z][a-z0-9_]{0,62}$/u.test(draft.key)) out.push('cmdb.group_key_invalid');
  if (!draft.name.trim()) out.push('cmdb.group_name_invalid');
  if (Object.keys(groupDefine(draft).filter).length === 0) out.push('cmdb.group_filter_empty');
  return out;
}

/** The service map's six layers, from what a person uses down to what everything stands on. */
export const MAP_LAYERS = [0, 1, 2, 3, 4, 5] as const;

type MapNode = ServiceMap['nodes'][number];

/**
 * The map's layers with their CIs in drawing order: each layer's CIs sorted by where their
 * neighbours in the layer above stand (the mean of their places), so lines cross less; a CI with
 * none above, and a tie, by name. The order is the same each time the same map is drawn.
 */
export function mapLayers(map: Pick<ServiceMap, 'nodes' | 'edges'>): { readonly layer: number; readonly nodes: readonly MapNode[] }[] {
  const neighbours = new Map<string, string[]>();
  for (const edge of map.edges) {
    neighbours.set(edge.source, [...(neighbours.get(edge.source) ?? []), edge.target]);
    neighbours.set(edge.target, [...(neighbours.get(edge.target) ?? []), edge.source]);
  }
  const place = new Map<string, number>();
  const out: { layer: number; nodes: MapNode[] }[] = [];
  for (const layer of MAP_LAYERS) {
    const here = map.nodes.filter((node) => node.layer === layer);
    if (here.length === 0) continue;
    const centre = (node: MapNode) => {
      const above = (neighbours.get(node.ci_id) ?? []).map((id) => place.get(id)).filter((at): at is number => at !== undefined);
      return above.length > 0 ? above.reduce((sum, at) => sum + at, 0) / above.length : Number.POSITIVE_INFINITY;
    };
    const ordered = here
      .map((node) => ({ node, centre: centre(node) }))
      .sort((a, b) => (a.centre === b.centre ? 0 : a.centre - b.centre) || a.node.name.localeCompare(b.node.name))
      .map((each) => each.node);
    ordered.forEach((node, index) => place.set(node.ci_id, index));
    out.push({ layer, nodes: ordered });
  }
  return out;
}

/** A map edge that joins a group's member to the service it stands for, not a relation. */
export const isGroupEdge = (type: string) => type.startsWith('group:');

// The asset register (UC-AST-12).

export const ASSET_STATES = ['REQUESTED', 'ORDERED', 'IN_STOCK', 'DEPLOYED', 'RETIRED', 'DISPOSED'] as const;
export type AssetState = (typeof ASSET_STATES)[number];
export const DISPOSAL_METHODS = ['WIPED', 'DESTROYED', 'RECYCLED', 'RETURNED', 'SOLD'] as const;
export type DisposalMethod = (typeof DISPOSAL_METHODS)[number];

/** Where each state may go, as the domain's `assetMoves` states it; disposed is the end. */
const ASSET_MOVES: Record<AssetState, readonly AssetState[]> = {
  REQUESTED: ['ORDERED', 'IN_STOCK'],
  ORDERED: ['IN_STOCK'],
  IN_STOCK: ['DEPLOYED', 'RETIRED'],
  DEPLOYED: ['IN_STOCK', 'RETIRED'],
  RETIRED: ['IN_STOCK', 'DISPOSED'],
  DISPOSED: [],
};
export const assetMoves = (state: AssetState): readonly AssetState[] => ASSET_MOVES[state] ?? [];

/** Whether a move is offered: deploying needs a holder or a stockroom (`cmdb.asset_deploy_unassigned`). */
export const moveNeedsHolder = (asset: Pick<CiAsset, 'assigned_to' | 'stockroom_id'>, to: AssetState) =>
  to === 'DEPLOYED' && !asset.assigned_to && !asset.stockroom_id;

/** How many minor units a currency's major one holds - two for EUR, none for JPY. */
export function minorDigits(currency: string): number {
  try {
    return new Intl.NumberFormat('en', { style: 'currency', currency }).resolvedOptions().maximumFractionDigits ?? 2;
  } catch {
    return 2;
  }
}

/** A typed cost in major units as minor units, or undefined when it is not a cost. */
export function parseCost(text: string, currency: string): number | undefined {
  const value = text.trim().replace(/\s/gu, '').replace(',', '.');
  if (!/^\d+(\.\d+)?$/u.test(value)) return undefined;
  return Math.round(Number(value) * 10 ** minorDigits(currency));
}

/** Minor units as the field shows them, in major units. */
export const costField = (minor: number, currency: string) => {
  const digits = minorDigits(currency);
  return (minor / 10 ** digits).toFixed(digits);
};

/** A cost in the reader's language, or a dash for none. */
export function costText(minor: number, currency: string, locale: string): string {
  if (!currency || minor === 0) return '—';
  try {
    return new Intl.NumberFormat(locale, { style: 'currency', currency }).format(minor / 10 ** minorDigits(currency));
  } catch {
    return `${costField(minor, currency)} ${currency}`;
  }
}

export interface AssetDraft {
  tag: string;
  cost: string;
  currency: string;
  purchased: string;
  supplier: string;
  warranty: string;
  contract: string;
  stockroom: string;
  holder: string;
  request: string;
  state: AssetState;
}

export function assetDraft(of?: CiAsset): AssetDraft {
  return {
    tag: of?.asset_tag ?? '',
    cost: of && of.currency && of.cost_minor ? costField(of.cost_minor, of.currency) : '',
    currency: of?.currency ?? '',
    purchased: of?.purchased_on ?? '',
    supplier: of?.supplier ?? '',
    warranty: of?.warranty_until ?? '',
    contract: of?.contract_id ?? '',
    stockroom: of?.stockroom_id ?? '',
    holder: of?.assigned_to ?? '',
    request: of?.request_id ?? '',
    state: of?.state ?? 'REQUESTED',
  };
}

/** What the form still lacks or holds wrong, as the server's codes, in the order of the fields. */
export function assetMissing(draft: AssetDraft): string[] {
  const out: string[] = [];
  const currency = draft.currency.trim().toUpperCase();
  if (draft.cost.trim() && parseCost(draft.cost, currency || 'EUR') === undefined) out.push('cmdb.asset_cost_invalid');
  if (draft.cost.trim() && !/^[A-Z]{3}$/u.test(currency)) out.push('cmdb.asset_currency_invalid');
  if (draft.purchased && draft.warranty && draft.warranty < draft.purchased) out.push('cmdb.asset_warranty_before_purchase');
  return out;
}

/**
 * The body `registerCiAsset` takes. A field emptied sends `null`, so the record forgets it; the
 * currency only beside a cost, which the contract's pattern needs; the state only for a new record,
 * because a record moves through `moveCiAsset`.
 */
export function assetRegister(draft: AssetDraft, isNew: boolean): CiAssetRegister {
  const currency = draft.currency.trim().toUpperCase();
  const cost = draft.cost.trim() ? parseCost(draft.cost, currency) ?? 0 : 0;
  const orNull = (value: string) => value.trim() || null;
  return {
    asset_tag: draft.tag.trim(),
    cost_minor: cost,
    ...(cost > 0 ? { currency } : {}),
    purchased_on: orNull(draft.purchased),
    supplier: draft.supplier.trim(),
    warranty_until: orNull(draft.warranty),
    contract_id: orNull(draft.contract),
    stockroom_id: orNull(draft.stockroom),
    assigned_to: orNull(draft.holder),
    request_id: orNull(draft.request),
    ...(isNew ? { state: draft.state } : {}),
  };
}

/** The tone an asset's state is drawn in; one retired or disposed is history. */
export function assetTone(asset: Pick<CiAsset, 'state' | 'warranty_ended'>): 'success' | 'info' | 'warning' | 'neutral' {
  if (asset.state === 'DEPLOYED') return asset.warranty_ended ? 'warning' : 'success';
  if (asset.state === 'IN_STOCK') return 'info';
  return 'neutral';
}

// Software (UC-AST-13).

/** The body `defineSoftwareAlias` takes; a match publisher left empty matches any publisher. */
export function aliasDefine(draft: { matchPublisher: string; matchProduct: string; publisher: string; product: string }): SoftwareAliasDefine {
  return {
    match_product: draft.matchProduct.trim(),
    product: draft.product.trim(),
    ...(draft.matchPublisher.trim() ? { match_publisher: draft.matchPublisher.trim() } : {}),
    ...(draft.publisher.trim() ? { publisher: draft.publisher.trim() } : {}),
  };
}

type Product = SoftwareProductList['data'][number];

/** The products whose publisher or product holds every word typed, the most installed first. */
export function matchingProducts(products: readonly Product[], term: string): Product[] {
  const words = term.toLocaleLowerCase().split(/\s+/u).filter(Boolean);
  return products
    .filter((each) => words.every((word) => `${each.publisher} ${each.product}`.toLocaleLowerCase().includes(word)))
    .sort((a, b) => b.installations - a.installations || a.product.localeCompare(b.product));
}
