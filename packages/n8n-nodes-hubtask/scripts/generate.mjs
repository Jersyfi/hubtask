// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Generates the n8n node from the contract (P-04, ADR-0058).
//
// n8n's declarative style is a description with routing rather than code per operation: a
// resource per tag, an operation per operationId with its method and path, a property per
// parameter and per body field with a `routing` that says where it goes. Everything below is
// read from `@hubtask/api-client`'s copy of the document, so a contract change changes the node
// in the same pull request - which is what "complete automatically" (automation.md §3.3) means.
//
// `--check` validates without writing, which is the package's typecheck: there is no
// `n8n-workflow` here to typecheck against (ADR-0058), and the schema in schema.mjs is what
// stands in for it.

import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { credentialProblems, describeProblems, selftest } from './schema.mjs';

const require = createRequire(import.meta.url);
const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

/* ── Reading the contract ──────────────────────────────────────────────────────────────── */

const METHODS = ['get', 'post', 'put', 'patch', 'delete'];

function refName(ref) {
  return ref ? ref.slice(ref.lastIndexOf('/') + 1) : undefined;
}

const ACRONYMS = new Set(['ai', 'id', 'url', 'oidc', 'mfa', 'totp', 'csv', 'ics', 'api', 'ics', 'mcp']);

/** A human name from an identifier: `createWorkItem` → `Create Work Item`, `WORK_PACKAGE` → `Work Package`, `ai` → `AI`. */
export function words(identifier) {
  const isConstant = /^[A-Z0-9_]+$/.test(identifier);
  const spaced = (isConstant ? identifier.toLowerCase() : identifier.replace(/([a-z0-9])([A-Z])/g, '$1 $2')).replace(/[_-]+/g, ' ');
  return spaced
    .split(' ')
    .filter(Boolean)
    .map((word) => (ACRONYMS.has(word.toLowerCase()) ? word.toUpperCase() : word.charAt(0).toUpperCase() + word.slice(1)))
    .join(' ');
}

function firstSentence(text = '') {
  const line = text.split('\n\n')[0].replace(/\s+/g, ' ').trim();
  const end = line.search(/[.!?](\s|$)/);
  return end === -1 ? line : line.slice(0, end + 1);
}

export function readDocument(document) {
  const parameters = document.components?.parameters ?? {};
  const schemas = document.components?.schemas ?? {};
  const resolveParameter = (p) => (p.$ref ? parameters[refName(p.$ref)] : p);
  const resolveSchema = (s) => (s?.$ref ? schemas[refName(s.$ref)] : s ?? {});

  /** Top-level fields of an object schema, allOf merged. */
  const fieldsOf = (schema) => {
    const resolved = resolveSchema(schema);
    const out = [];
    const required = new Set(resolved.required ?? []);
    const parts = resolved.allOf ? resolved.allOf.map(resolveSchema) : [resolved];
    for (const part of parts) {
      for (const name of part.required ?? []) required.add(name);
      for (const [name, property] of Object.entries(part.properties ?? {})) {
        const target = resolveSchema(property);
        const types = Array.isArray(target.type) ? target.type : target.type ? [target.type] : [];
        const own = types.find((t) => t !== 'null') ?? (target.properties || target.allOf ? 'object' : 'string');
        out.push({ name, required: required.has(name), type: own, enum: target.enum, description: firstSentence(property.description ?? target.description) });
      }
    }
    return out;
  };

  const operations = [];
  for (const [route, item] of Object.entries(document.paths)) {
    const shared = Array.isArray(item.parameters) ? item.parameters : [];
    for (const [method, op] of Object.entries(item)) {
      if (!METHODS.includes(method)) continue;
      const all = [...shared, ...(op.parameters ?? [])].map(resolveParameter);
      const body = op.requestBody?.content ? Object.entries(op.requestBody.content)[0] : undefined;
      const isJsonBody = body && (body[0] === 'application/json' || body[0].endsWith('+json'));
      operations.push({
        id: op.operationId ?? '',
        tag: op.tags?.[0] ?? 'other',
        method: method.toUpperCase(),
        path: route,
        summary: op.summary ?? words(op.operationId ?? `${method} ${route}`),
        public: Array.isArray(op.security) && op.security.length === 0,
        pathParameters: all.filter((p) => p.in === 'path'),
        queryParameters: all.filter((p) => p.in === 'query'),
        headerParameters: all.filter((p) => p.in === 'header'),
        body: body ? { contentType: body[0], json: !!isJsonBody, fields: isJsonBody ? fieldsOf(body[1].schema) : [] } : undefined,
      });
    }
  }
  const tags = [...new Set(operations.map((op) => op.tag))];
  return { title: document.info.title, operations, tags };
}

/* ── The node description ──────────────────────────────────────────────────────────────── */

const N8N_TYPE = { string: 'string', integer: 'number', number: 'number', boolean: 'boolean', array: 'json', object: 'json' };

function n8nField(field) {
  const type = field.enum && field.type === 'string' ? 'options' : (N8N_TYPE[field.type] ?? 'string');
  const property = {
    displayName: words(field.name),
    name: field.name,
    type,
    default: type === 'boolean' ? false : type === 'number' ? 0 : type === 'json' ? '{}' : type === 'options' ? field.enum.filter((v) => v !== null)[0] : '',
    description: field.description,
  };
  if (type === 'options') property.options = field.enum.filter((v) => v !== null).map((value) => ({ name: words(String(value)), value }));
  if (type === 'json') property.routing = { send: { type: 'body', property: field.name, value: `={{ JSON.parse($value) }}` } };
  else property.routing = { send: { type: 'body', property: field.name } };
  return property;
}

function n8nQuery(parameter) {
  const schema = parameter.schema ?? {};
  const type = schema.enum ? 'options' : (N8N_TYPE[schema.type] ?? 'string');
  const property = {
    displayName: words(parameter.name),
    name: parameter.name,
    type,
    default: type === 'boolean' ? false : type === 'number' ? 0 : type === 'options' ? schema.enum[0] : '',
    description: firstSentence(parameter.description),
    routing: { send: { type: 'query', property: parameter.name } },
  };
  if (type === 'options') property.options = schema.enum.map((value) => ({ name: words(String(value)), value }));
  return property;
}

/** The URL with its path parameters as n8n expressions over the properties. */
function routeExpression(route) {
  const expression = route.replace(/\{([^}]+)\}/g, (_, name) => `{{ encodeURIComponent($parameter["${name}"]) }}`);
  return expression === route ? route : `=${expression}`;
}

export function describeNode(contract) {
  const properties = [];
  properties.push({
    displayName: 'Resource',
    name: 'resource',
    type: 'options',
    noDataExpression: true,
    options: contract.tags.map((tag) => ({ name: words(tag), value: tag })),
    default: contract.tags[0],
  });
  for (const tag of contract.tags) {
    const ops = contract.operations.filter((op) => op.tag === tag);
    properties.push({
      displayName: 'Operation',
      name: 'operation',
      type: 'options',
      noDataExpression: true,
      displayOptions: { show: { resource: [tag] } },
      options: ops.map((op) => ({
        name: op.summary,
        value: op.id,
        action: op.summary,
        description: `${op.method} ${op.path}`,
        routing: { request: { method: op.method, url: routeExpression(op.path) } },
      })),
      default: ops[0].id,
    });
    for (const op of ops) {
      const show = { resource: [tag], operation: [op.id] };
      for (const p of op.pathParameters) {
        properties.push({ displayName: words(p.name), name: p.name, type: 'string', required: true, default: '', description: firstSentence(p.description), displayOptions: { show } });
      }
      if (op.body?.json) {
        const required = op.body.fields.filter((f) => f.required);
        const optional = op.body.fields.filter((f) => !f.required);
        for (const field of required) properties.push({ ...n8nField(field), required: true, displayOptions: { show } });
        if (optional.length > 0) {
          properties.push({
            displayName: 'Additional Fields',
            name: 'additionalFields',
            type: 'collection',
            placeholder: 'Add Field',
            default: {},
            displayOptions: { show },
            options: optional.map(n8nField),
          });
        }
      } else if (op.body) {
        properties.push({
          displayName: 'Body',
          name: 'body',
          type: 'string',
          default: '',
          description: `Sent as ${op.body.contentType}`,
          displayOptions: { show },
          routing: { request: { body: '={{ $value }}', headers: { 'Content-Type': op.body.contentType } } },
        });
      }
      if (op.queryParameters.length > 0) {
        properties.push({
          displayName: 'Options',
          name: 'options',
          type: 'collection',
          placeholder: 'Add Option',
          default: {},
          displayOptions: { show },
          options: op.queryParameters.map(n8nQuery),
        });
      }
      for (const h of op.headerParameters) {
        if (h.name === 'Idempotency-Key') continue; // n8n retries are the person's; a key per run is theirs to set
        properties.push({
          displayName: words(h.name),
          name: h.name.replace(/-/g, ''),
          type: 'string',
          default: '',
          description: firstSentence(h.description),
          displayOptions: { show },
          routing: { request: { headers: { [h.name]: '={{ $value }}' } } },
        });
      }
    }
  }
  return {
    displayName: 'Hubtask',
    name: 'hubtask',
    icon: 'fa:tasks',
    group: ['transform'],
    version: 1,
    subtitle: '={{ $parameter["operation"] + ": " + $parameter["resource"] }}',
    description: `${contract.title}: every operation of the contract`,
    defaults: { name: 'Hubtask' },
    inputs: ['main'],
    outputs: ['main'],
    credentials: [{ name: 'hubtaskApi', required: true }],
    requestDefaults: {
      baseURL: '={{ $credentials.baseUrl.replace(/\\/$/, "") }}',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    },
    properties,
  };
}

export function describeCredential() {
  return {
    name: 'hubtaskApi',
    displayName: 'Hubtask API',
    documentationUrl: 'https://hubtask.eu/developers/api/',
    properties: [
      { displayName: 'Base URL', name: 'baseUrl', type: 'string', default: 'https://hubtask.example/api/v1', description: 'The installation\'s API root, ending in /api/v1', required: true },
      { displayName: 'Personal Access Token', name: 'token', type: 'string', typeOptions: { password: true }, default: '', description: 'A personal access token (hbt_pat_…) or a service account token', required: true },
    ],
    authenticate: { type: 'generic', properties: { headers: { Authorization: '=Bearer {{$credentials.token}}' } } },
    test: { request: { baseURL: '={{ $credentials.baseUrl.replace(/\\/$/, "") }}', url: '/accounts/me' } },
  };
}

/* ── Writing ───────────────────────────────────────────────────────────────────────────── */

/** The manifest of the package as it would be published; `private` until the owner does. */
export function publishedManifest() {
  const own = require('../package.json');
  return {
    // The registry's convention: a community node package is `n8n-nodes-<name>`, unscoped. The
    // workspace's own name carries the `@hubtask` scope every member here has.
    name: 'n8n-nodes-hubtask',
    version: own.version,
    private: true,
    description: 'The n8n community node for Hubtask, generated from the OpenAPI contract',
    license: own.license,
    keywords: ['n8n-community-node-package'],
    main: 'index.js',
    files: ['nodes', 'credentials', 'index.js', 'README.md'],
    n8n: {
      n8nNodesApiVersion: 1,
      credentials: ['credentials/HubtaskApi.credentials.js'],
      nodes: ['nodes/Hubtask/Hubtask.node.js', 'nodes/HubtaskTrigger/HubtaskTrigger.node.js'],
    },
    peerDependencies: { 'n8n-workflow': '>=1.0.0' },
  };
}

export function generate({ document, events }) {
  const contract = readDocument(document);
  const description = describeNode(contract);
  const credential = describeCredential();
  const eventTypes = Object.keys(events).sort();
  const problems = [...describeProblems(description), ...credentialProblems(credential)];
  return { contract, description, credential, eventTypes, problems };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  if (!selftest()) {
    console.error('n8n: the format validator no longer catches what it claims to');
    process.exit(1);
  }
  const document = require('@hubtask/api-client/openapi.json');
  const events = require('@hubtask/api-client/events.json');
  const { contract, description, credential, eventTypes, problems } = generate({ document, events });
  if (problems.length > 0) {
    for (const problem of problems) console.error(`n8n: ${problem}`);
    process.exit(1);
  }
  if (process.argv.includes('--check')) {
    console.log(`n8n: ${contract.operations.length} operations under ${contract.tags.length} resources and ${eventTypes.length} event types validate`);
    process.exit(0);
  }
  const dist = path.join(packageRoot, 'dist');
  fs.rmSync(dist, { recursive: true, force: true });
  fs.mkdirSync(path.join(dist, 'nodes', 'Hubtask'), { recursive: true });
  fs.mkdirSync(path.join(dist, 'nodes', 'HubtaskTrigger'), { recursive: true });
  fs.mkdirSync(path.join(dist, 'credentials'), { recursive: true });
  fs.writeFileSync(path.join(dist, 'nodes', 'Hubtask', 'Hubtask.node.json'), JSON.stringify(description, null, 2) + '\n');
  fs.writeFileSync(path.join(dist, 'nodes', 'HubtaskTrigger', 'events.json'), JSON.stringify(eventTypes, null, 2) + '\n');
  fs.writeFileSync(path.join(dist, 'credentials', 'HubtaskApi.credentials.json'), JSON.stringify(credential, null, 2) + '\n');
  // The manifest n8n and the registry read is dist/package.json: the workspace's own manifest
  // names no platform library, because autoInstallPeers would put `n8n-workflow` and its tree
  // into this repository's lockfile (ADR-0058, option A). What is published is dist/.
  fs.writeFileSync(path.join(dist, 'package.json'), JSON.stringify(publishedManifest(), null, 2) + '\n');
  fs.copyFileSync(path.join(packageRoot, 'index.js'), path.join(dist, 'index.js'));
  fs.copyFileSync(path.join(packageRoot, 'README.md'), path.join(dist, 'README.md'));
  // The three loaders are hand-written and copied: n8n resolves the paths the manifest names.
  fs.copyFileSync(path.join(packageRoot, 'nodes', 'Hubtask', 'Hubtask.node.js'), path.join(dist, 'nodes', 'Hubtask', 'Hubtask.node.js'));
  fs.copyFileSync(path.join(packageRoot, 'nodes', 'HubtaskTrigger', 'HubtaskTrigger.node.js'), path.join(dist, 'nodes', 'HubtaskTrigger', 'HubtaskTrigger.node.js'));
  fs.copyFileSync(path.join(packageRoot, 'credentials', 'HubtaskApi.credentials.js'), path.join(dist, 'credentials', 'HubtaskApi.credentials.js'));
  console.log(`n8n: ${contract.operations.length} operations under ${contract.tags.length} resources, ${eventTypes.length} event types -> dist/`);
}
