// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Generates the Zapier app from the contract (P-05, ADR-0058).
//
// Zapier's format is code per trigger, create and search, so the generator writes one file per
// entry over one hand-written request helper (src/hubtask.js): a trigger per event type as REST
// hooks - subscribe, unsubscribe, and `performList` through trigger polling for the sample - the
// creates the marketplace's review expects, and the two searches. The samples are built from the
// contract's schemas, because a trigger or a create without one is refused at review.
//
// `--check` generates into a temporary directory, loads the app and validates it against
// scripts/schema.mjs - the package's typecheck, since `zapier-platform-core` is not installed
// here (ADR-0058).

import { createRequire } from 'node:module';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { appProblems, selftest } from './schema.mjs';

const require = createRequire(import.meta.url);
const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

/**
 * The platform library's version, pinned to the platform's current one: `zapier validate` names
 * the latest under D027, and `cleanInputData` - which every perform here needs said, because they
 * drop empty values themselves - is a key the schema learned after 17 (issue 722).
 */
export const PLATFORM_VERSION = '19.1.0';

/** The creates the app offers: the writes a Zap does (automation.md §3.3), by operationId. */
export const CREATES = [
  { id: 'createWorkItem', noun: 'Entry', label: 'Create Entry' },
  { id: 'updateWorkItem', noun: 'Entry', label: 'Update Entry' },
  { id: 'completeWorkItem', noun: 'Entry', label: 'Complete Entry' },
  { id: 'addComment', noun: 'Comment', label: 'Add Comment' },
  { id: 'createContainer', noun: 'Collection', label: 'Create Hub or Collection' },
];

/** The searches: text, and a filter (automation.md §3.3). */
export const SEARCHES = [
  { id: 'searchItems', noun: 'Entry', label: 'Find Entry by Text' },
  { id: 'queryItems', noun: 'Entry', label: 'Find Entries by Filter' },
];

/**
 * The hidden triggers that feed a dynamic dropdown (the platform's D004): an identifier field is
 * a list to choose from, not a UUID to paste. Each one is a listing of the contract, read by
 * `operationId`; one that `needs` a field of the form reads it from the form's current input and
 * answers nothing until it is filled. There is no listing for accounts by design - a membership
 * names an account by identifier only - so an assignee stays a field one fills from a search.
 */
export const CHOICES = [
  { key: 'hubChoices', noun: 'Hub', operation: 'listContainers', query: { type: 'HUB' }, label: 'name' },
  { key: 'collectionChoices', noun: 'Collection', operation: 'listContainers', query: { type: 'COLLECTION' }, label: 'name' },
  { key: 'containerChoices', noun: 'Container', operation: 'listContainers', query: {}, label: 'name' },
  { key: 'entryChoices', noun: 'Entry', operation: 'listWorkItems', needs: 'collection_id', query: { collection_id: '{collection_id}' }, label: 'title' },
  // A form that names an entry and no collection - an update - reaches the buckets through the
  // entry's own collection, one read away.
  { key: 'bucketChoices', noun: 'Bucket', operation: 'listBuckets', needs: 'collection_id', via: { field: 'itemId', operation: 'getWorkItem', read: 'collection_id' }, path: { containerId: '{collection_id}' }, label: 'name' },
  { key: 'commentChoices', noun: 'Comment', operation: 'listComments', needs: 'itemId', path: { itemId: '{itemId}' }, label: 'body' },
];

/** Which field of which entry is fed by which choices trigger, as `key.id.label`. */
export const DROPDOWNS = {
  createWorkItem: { collection_id: 'collectionChoices.id.name', parent_id: 'entryChoices.id.title', before_item_id: 'entryChoices.id.title', bucket_id: 'bucketChoices.id.name' },
  updateWorkItem: { bucket_id: 'bucketChoices.id.name' },
  addComment: { parent_comment_id: 'commentChoices.id.body' },
  createContainer: { parent_id: 'hubChoices.id.name' },
  searchItems: { container_id: 'containerChoices.id.name' },
};

/** The fields a search step can fill: an entry's identifier, wherever a form asks for one. */
export const SEARCHABLE = { itemId: 'searchItems.id', parent_id: 'searchItems.id', before_item_id: 'searchItems.id' };

/* ── Reading the contract ──────────────────────────────────────────────────────────────── */

function refName(ref) {
  return ref ? ref.slice(ref.lastIndexOf('/') + 1) : undefined;
}

export function words(identifier) {
  const isConstant = /^[A-Z0-9_]+$/.test(identifier);
  const spaced = (isConstant ? identifier.toLowerCase() : identifier.replace(/([a-z0-9])([A-Z])/g, '$1 $2')).replace(/[_.-]+/g, ' ');
  return spaced.split(' ').filter(Boolean).map((w) => (['id', 'ai', 'url'].includes(w) ? w.toUpperCase() : w.charAt(0).toUpperCase() + w.slice(1))).join(' ');
}

function firstSentence(text = '') {
  const line = text.split('\n\n')[0].replace(/\s+/g, ' ').trim();
  const end = line.search(/[.!?](\s|$)/);
  return end === -1 ? line : line.slice(0, end + 1);
}

/** `de.hubtask.work.item.created.v1` → `workItemCreated`. */
export function triggerKey(eventType) {
  const parts = eventType.replace(/^de\.hubtask\./, '').replace(/\.v\d+$/, '').split('.');
  return parts.map((part, i) => (i === 0 ? part : part.charAt(0).toUpperCase() + part.slice(1))).join('').replace(/_(\w)/g, (_, c) => c.toUpperCase());
}

export function readDocument(document) {
  const parameters = document.components?.parameters ?? {};
  const schemas = document.components?.schemas ?? {};
  const resolveSchema = (s) => (s?.$ref ? schemas[refName(s.$ref)] : s ?? {});
  const resolveParameter = (p) => (p.$ref ? parameters[refName(p.$ref)] : p);

  const fieldsOf = (schema) => {
    const resolved = resolveSchema(schema);
    const required = new Set(resolved.required ?? []);
    const parts = resolved.allOf ? resolved.allOf.map(resolveSchema) : [resolved];
    const out = [];
    for (const part of parts) {
      for (const name of part.required ?? []) required.add(name);
      for (const [name, property] of Object.entries(part.properties ?? {})) {
        const target = resolveSchema(property);
        const types = Array.isArray(target.type) ? target.type : target.type ? [target.type] : [];
        const own = types.find((t) => t !== 'null') ?? (target.properties || target.allOf ? 'object' : 'string');
        out.push({ name, required: required.has(name), type: own, enum: target.enum?.filter((v) => v !== null), description: firstSentence(property.description ?? target.description) });
      }
    }
    return out;
  };

  const exampleOf = (schema, depth = 0) => {
    const resolved = resolveSchema(schema);
    if (resolved.example !== undefined) return resolved.example;
    if (schema?.example !== undefined) return schema.example;
    if (resolved.default !== undefined) return resolved.default;
    if (resolved.enum) return resolved.enum.find((v) => v !== null);
    if (resolved.allOf) return Object.assign({}, ...resolved.allOf.map((part) => exampleOf(part, depth)));
    const union = resolved.oneOf ?? resolved.anyOf;
    if (union) return exampleOf(union[0], depth);
    const types = Array.isArray(resolved.type) ? resolved.type : resolved.type ? [resolved.type] : [];
    const type = types.find((t) => t !== 'null') ?? (resolved.properties ? 'object' : 'string');
    switch (type) {
      case 'object': {
        if (depth >= 3) return {};
        const out = {};
        for (const [name, property] of Object.entries(resolved.properties ?? {})) out[name] = exampleOf(property, depth + 1);
        return out;
      }
      case 'array': return depth >= 3 ? [] : [exampleOf(resolved.items ?? {}, depth + 1)];
      case 'integer': return 1;
      case 'number': return 1.5;
      case 'boolean': return true;
      default:
        switch (resolved.format) {
          case 'uuid': return '018f6e2a-2b7c-7c1e-9b1a-3d5e6f7a8b9c';
          case 'date-time': return '2026-09-16T10:00:00Z';
          case 'date': return '2026-09-16';
          case 'email': return 'anna@example.org';
          case 'uri': return 'https://example.org';
          default: return 'text';
        }
    }
  };

  const operations = new Map();
  for (const [route, item] of Object.entries(document.paths)) {
    const shared = Array.isArray(item.parameters) ? item.parameters : [];
    for (const [method, op] of Object.entries(item)) {
      if (!['get', 'post', 'put', 'patch', 'delete'].includes(method)) continue;
      const all = [...shared, ...(op.parameters ?? [])].map(resolveParameter);
      const body = op.requestBody?.content ? Object.entries(op.requestBody.content)[0] : undefined;
      const answer = Object.entries(op.responses ?? {}).find(([status]) => status.startsWith('2'));
      const answerSchema = answer?.[1]?.content ? Object.values(answer[1].content)[0]?.schema : undefined;
      operations.set(op.operationId, {
        id: op.operationId,
        method: method.toUpperCase(),
        path: route,
        summary: op.summary ?? words(op.operationId ?? ''),
        description: firstSentence(op.description) || op.summary || words(op.operationId ?? ''),
        pathParameters: all.filter((p) => p.in === 'path'),
        headerParameters: all.filter((p) => p.in === 'header'),
        body: body ? { contentType: body[0], fields: fieldsOf(body[1].schema) } : undefined,
        sample: answerSchema ? exampleOf(answerSchema) : {},
      });
    }
  }
  return { title: document.info.title, operations, exampleOf, fieldsOf };
}

/* ── The entries ───────────────────────────────────────────────────────────────────────── */

const ZAPIER_TYPE = { string: 'string', integer: 'integer', number: 'number', boolean: 'boolean' };

function inputField(field, entryId) {
  const out = { key: field.name, label: words(field.name), required: field.required, helpText: field.description || undefined };
  if (field.enum && field.type === 'string') out.choices = field.enum;
  else if (field.type === 'array') out.list = true;
  else if (field.type === 'object') out.dict = true;
  else out.type = ZAPIER_TYPE[field.type] ?? 'string';
  if (field.name === 'notes' || field.name === 'body') out.type = 'text';
  return withChoices(out, entryId);
}

function pathField(parameter, entryId) {
  return withChoices({ key: parameter.name, label: words(parameter.name), required: true, type: 'string', helpText: firstSentence(parameter.description) || undefined }, entryId);
}

/** The dropdown and the search step a field takes, where one is declared for it. */
function withChoices(field, entryId) {
  const dynamic = DROPDOWNS[entryId]?.[field.key];
  if (dynamic) field.dynamic = dynamic;
  // A field another dropdown depends on refreshes the form when it changes.
  if (CHOICES.some((choice) => choice.needs === field.key || choice.via?.field === field.key)) field.altersDynamicFields = true;
  if (SEARCHABLE[field.key] && entryId !== 'searchItems') field.search = SEARCHABLE[field.key];
  return field;
}

const HEADER = '// Code generated by scripts/generate.mjs from api/openapi.yaml - DO NOT EDIT.\n//\n// SPDX-License-Identifier: Apache-2.0\n\'use strict\';\n\nconst { request } = require(\'../hubtask\');\n\n';

function js(value) {
  return JSON.stringify(value, null, 2);
}

export function createSource(op, entry) {
  const fields = [...op.pathParameters.map((p) => pathField(p, entry.id)), ...(op.body?.fields ?? []).map((f) => inputField(f, entry.id))];
  const bodyKeys = (op.body?.fields ?? []).map((f) => f.name);
  const pathKeys = op.pathParameters.map((p) => p.name);
  const contentType = op.body?.contentType;
  return `${HEADER}const PATH_KEYS = ${js(pathKeys)};
const BODY_KEYS = ${js(bodyKeys)};

function fill(template, input) {
  return template.replace(/\\{([^}]+)\\}/g, (_, name) => encodeURIComponent(String(input[name] ?? '')));
}

module.exports = {
  key: ${js(entry.id)},
  noun: ${js(entry.noun)},
  display: { label: ${js(entry.label)}, description: ${js(op.description)} },
  operation: {
    inputFields: ${js(fields)},
    perform: async (z, bundle) => {
      const body = {};
      for (const key of BODY_KEYS) if (bundle.inputData[key] !== undefined && bundle.inputData[key] !== '') body[key] = bundle.inputData[key];
      return request(z, bundle, ${js(op.method)}, fill(${js(op.path)}, bundle.inputData), {
        body: BODY_KEYS.length > 0 ? body : undefined,
        headers: { 'Content-Type': ${js(contentType ?? 'application/json')}, 'Idempotency-Key': z.hash('md5', JSON.stringify([bundle.inputData, bundle.meta && bundle.meta.zap && bundle.meta.zap.id])) },
      });
    },
    sample: ${js(op.sample)},
  },
};
`;
}

export function searchSource(op, entry) {
  const fields = (op.body?.fields ?? []).map((f) => inputField(f, entry.id));
  return `${HEADER}module.exports = {
  key: ${js(entry.id)},
  noun: ${js(entry.noun)},
  display: { label: ${js(entry.label)}, description: ${js(op.description)} },
  operation: {
    inputFields: ${js(fields)},
    perform: async (z, bundle) => {
      const body = {};
      for (const [key, value] of Object.entries(bundle.inputData)) if (value !== undefined && value !== '') body[key] = value;
      const page = await request(z, bundle, ${js(op.method)}, ${js(op.path)}, { body, headers: { 'Content-Type': 'application/json' } });
      return Array.isArray(page.data) ? page.data : [];
    },
    sample: ${js(op.sample?.data?.[0] ?? op.sample)},
  },
};
`;
}

/**
 * A trigger's description, in the shape the platform's D021 check demands: "Triggers when " and
 * then the event's own first sentence, as the contract writes it in api/events - "Triggers when a
 * task, a work package or an activity was created." The task references the schemas carry for a
 * reader of the repository ("(C-03)") are dropped; a person building a Zap has no use for them.
 */
export function triggerDescription(eventType, schema) {
  const own = firstSentence(schema?.description ?? '')
    .replace(/\s*\((?:[A-Z]{1,2}-\d+|invariant [^)]*)\)/g, '')
    .replace(/\s+([.,:;])/g, '$1')
    .trim();
  if (own === '') return `Triggers when ${eventType} happens in the connected workspace.`;
  const sentence = own.charAt(0).toLowerCase() + own.slice(1);
  return `Triggers when ${/[.!?]$/.test(sentence) ? sentence : `${sentence}.`}`;
}

export function triggerSource(eventType, sample, schema) {
  const key = triggerKey(eventType);
  return `${HEADER}const EVENT_TYPE = ${js(eventType)};

module.exports = {
  key: ${js(key)},
  noun: 'Event',
  display: { label: ${js(words(triggerKey(eventType)))}, description: ${js(triggerDescription(eventType, schema))} },
  operation: {
    type: 'hook',
    inputFields: [],
    performSubscribe: async (z, bundle) =>
      request(z, bundle, 'POST', '/integrations/webhooks', {
        body: { target_url: bundle.targetUrl, event_types: [EVENT_TYPE] },
        headers: { 'Content-Type': 'application/json' },
      }),
    performUnsubscribe: async (z, bundle) =>
      request(z, bundle, 'DELETE', '/integrations/webhooks/' + encodeURIComponent(bundle.subscribeData.id)),
    perform: (z, bundle) => [bundle.cleanedRequest],
    performList: async (z, bundle) => {
      const page = await request(z, bundle, 'GET', '/integrations/triggers/' + encodeURIComponent(EVENT_TYPE), { query: { limit: 3 } });
      return Array.isArray(page.data) ? page.data : Array.isArray(page) ? page : [];
    },
    sample: ${js(sample)},
  },
};
`;
}

/**
 * A choices trigger: a polling trigger the platform never shows, read for a dropdown. It answers
 * the first page at the contract's largest size, because a dropdown is not a paginated list; a
 * choice that `needs` a field answers nothing until the form has it.
 */
export function choicesSource(choice, op, contract) {
  const query = { ...choice.query, size: 200 };
  const sample = op.sample?.data?.[0] ?? (Array.isArray(op.sample) ? op.sample[0] : op.sample) ?? {};
  const route = op.path.replace(/\{[^}]+\}/g, (m) => (choice.path ? choice.path[m.slice(1, -1)] ?? m : m));
  const via = choice.via ? contract.operations.get(choice.via.operation) : undefined;
  if (choice.via && !via) throw new Error(`zapier: the contract has no operation ${choice.via.operation}`);
  const resolve = choice.via
    ? `if (!input[${js(choice.needs)}] && input[${js(choice.via.field)}]) {
        const found = await request(z, bundle, ${js(via.method)}, fill(${js(via.path)}, input));
        input = { ...input, [${js(choice.needs)}]: found[${js(choice.via.read)}] };
      }
      `
    : '';
  return `${HEADER}function fill(template, input) {
  return template.replace(/\\{([^}]+)\\}/g, (_, name) => encodeURIComponent(String(input[name] ?? '')));
}

module.exports = {
  key: ${js(choice.key)},
  noun: ${js(choice.noun)},
  display: { label: ${js(words(choice.key))}, description: ${js(`Triggers when the ${choice.noun.toLowerCase()} choices are read for a dropdown.`)}, hidden: true },
  operation: {
    type: 'polling',
    inputFields: [],
    perform: async (z, bundle) => {
      let input = bundle.inputData || {};
      ${resolve}${choice.needs ? `if (!input[${js(choice.needs)}]) return [];\n      ` : ''}const query = {};
      for (const [key, value] of Object.entries(${js(query)})) query[key] = typeof value === 'string' ? value.replace(/\\{([^}]+)\\}/g, (_, name) => String(input[name] ?? '')) : value;
      const page = await request(z, bundle, ${js(op.method)}, fill(${js(route)}, input), { query });
      return Array.isArray(page.data) ? page.data : Array.isArray(page) ? page : [];
    },
    sample: ${js(sample)},
  },
};
`;
}

/** The sample delivery for an event type: the CloudEvents envelope with an example payload. */
export function eventSample(eventType, schema, exampleOf) {
  const data = schema.properties?.data ? exampleOf(schema.properties.data) : {};
  return {
    specversion: '1.0',
    id: '018f6e2a-2b7c-7c1e-9b1a-3d5e6f7a8b9c',
    type: eventType,
    source: 'https://hubtask.example/api/v1',
    time: '2026-09-16T10:00:00Z',
    datacontenttype: 'application/json',
    data,
  };
}

export function indexSource(triggers, creates, searches, version) {
  const line = (dir, key) => `    ${js(key)}: require('./${dir}/${key}'),`;
  return `${HEADER.replace("const { request } = require('../hubtask');\n\n", '')}const authentication = require('./authentication');

module.exports = {
  version: ${js(version)},
  platformVersion: ${js(PLATFORM_VERSION)},
  authentication,
  // Every perform drops the empty values itself, and says so here rather than leaving the
  // platform to do it first (D028): what reaches a perform is what the person entered.
  flags: { cleanInputData: false },
  beforeRequest: [
    (request, z, bundle) => {
      if (bundle.authData && bundle.authData.access_token) request.headers.Authorization = 'Bearer ' + bundle.authData.access_token;
      return request;
    },
  ],
  triggers: {
${triggers.map((key) => line('triggers', key)).join('\n')}
  },
  creates: {
${creates.map((key) => line('creates', key)).join('\n')}
  },
  searches: {
${searches.map((key) => line('searches', key)).join('\n')}
  },
};
`;
}

/** The manifest of the app as it would be pushed with the Zapier CLI; `private` until the owner does. */
export function publishedManifest(version) {
  return {
    name: 'hubtask-zapier',
    version,
    private: true,
    description: 'The Zapier app for Hubtask, generated from the OpenAPI contract',
    license: 'Apache-2.0',
    main: 'index.js',
    // `zapier-platform-core` is a dependency of a Zapier app by the CLI's own rule, at the exact
    // version the platform pins; it is named here, in the manifest the CLI reads, and not in the
    // workspace's own (ADR-0058).
    dependencies: { 'zapier-platform-core': PLATFORM_VERSION },
    zapier: { convertedByCLIVersion: PLATFORM_VERSION },
  };
}

/* ── Writing ───────────────────────────────────────────────────────────────────────────── */

export function writeApp({ document, events, into }) {
  const contract = readDocument(document);
  const version = require('../package.json').version;
  fs.rmSync(into, { recursive: true, force: true });
  for (const dir of ['triggers', 'creates', 'searches']) fs.mkdirSync(path.join(into, dir), { recursive: true });
  const triggers = [];
  for (const [type, schema] of Object.entries(events).sort()) {
    const key = triggerKey(type);
    fs.writeFileSync(path.join(into, 'triggers', `${key}.js`), triggerSource(type, eventSample(type, schema, contract.exampleOf), schema));
    triggers.push(key);
  }
  for (const choice of CHOICES) {
    const op = contract.operations.get(choice.operation);
    if (!op) throw new Error(`zapier: the contract has no operation ${choice.operation}`);
    fs.writeFileSync(path.join(into, 'triggers', `${choice.key}.js`), choicesSource(choice, op, contract));
    triggers.push(choice.key);
  }
  for (const entry of CREATES) {
    const op = contract.operations.get(entry.id);
    if (!op) throw new Error(`zapier: the contract has no operation ${entry.id}`);
    fs.writeFileSync(path.join(into, 'creates', `${entry.id}.js`), createSource(op, entry));
  }
  for (const entry of SEARCHES) {
    const op = contract.operations.get(entry.id);
    if (!op) throw new Error(`zapier: the contract has no operation ${entry.id}`);
    fs.writeFileSync(path.join(into, 'searches', `${entry.id}.js`), searchSource(op, entry));
  }
  fs.writeFileSync(path.join(into, 'index.js'), indexSource(triggers, CREATES.map((c) => c.id), SEARCHES.map((s) => s.id), version));
  fs.writeFileSync(path.join(into, 'package.json'), JSON.stringify(publishedManifest(version), null, 2) + '\n');
  for (const file of ['hubtask.js', 'authentication.js']) fs.copyFileSync(path.join(packageRoot, 'src', file), path.join(into, file));
  fs.copyFileSync(path.join(packageRoot, 'README.md'), path.join(into, 'README.md'));
  // The app is Apache-2.0 (ADR-0059 §6), and what is pushed carries its licence file.
  fs.copyFileSync(path.join(packageRoot, 'LICENSE'), path.join(into, 'LICENSE'));
  return { triggers, creates: CREATES.map((c) => c.id), searches: SEARCHES.map((s) => s.id) };
}

/** Load the written app fresh, so that a check reads what was written and not a cached module. */
export function loadApp(dir) {
  const entry = path.join(dir, 'index.js');
  for (const key of Object.keys(require.cache)) if (key.startsWith(dir)) delete require.cache[key];
  return require(entry);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  if (!selftest()) {
    console.error('zapier: the format validator no longer catches what it claims to');
    process.exit(1);
  }
  const document = require('@hubtask/api-client/openapi.json');
  const events = require('@hubtask/api-client/events.json');
  const checking = process.argv.includes('--check');
  const into = checking ? fs.mkdtempSync(path.join(os.tmpdir(), 'hubtask-zapier-')) : path.join(packageRoot, 'dist');
  const written = writeApp({ document, events, into });
  const problems = appProblems(loadApp(into));
  if (checking) fs.rmSync(into, { recursive: true, force: true });
  if (problems.length > 0) {
    for (const problem of problems) console.error(`zapier: ${problem}`);
    process.exit(1);
  }
  console.log(`zapier: ${written.triggers.length} triggers, ${written.creates.length} creates, ${written.searches.length} searches ${checking ? 'validate' : '-> dist/'}`);
}
