// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The shape of an n8n declarative node description, as n8n's documentation for community nodes
// describes it, written as a validator (ADR-0058, option A). This is what stands in for a
// typecheck against `n8n-workflow`, which this workspace does not install: it catches a property
// without a name, an option without a value, a routing without a request, and the mistakes a
// generator makes - but not a property n8n renamed last week. That one the walk in P-17 catches,
// by loading the package into a real n8n.

const PROPERTY_TYPES = new Set(['string', 'number', 'boolean', 'options', 'multiOptions', 'collection', 'fixedCollection', 'json', 'dateTime', 'notice', 'hidden']);
const METHODS = new Set(['GET', 'POST', 'PUT', 'PATCH', 'DELETE']);

function expect(condition, path, message, problems) {
  if (!condition) problems.push(`${path}: ${message}`);
}

function checkRouting(routing, path, problems) {
  if (routing === undefined) return;
  expect(typeof routing === 'object' && routing !== null, path, 'routing is not an object', problems);
  if (routing.request) {
    const { method, url, body, headers } = routing.request;
    if (method !== undefined) expect(METHODS.has(method), `${path}.request.method`, `unknown method ${method}`, problems);
    if (url !== undefined) expect(typeof url === 'string' && url.length > 0, `${path}.request.url`, 'url is not a string', problems);
    if (body !== undefined) expect(typeof body === 'string' || typeof body === 'object', `${path}.request.body`, 'body is neither an expression nor an object', problems);
    if (headers !== undefined) expect(typeof headers === 'object', `${path}.request.headers`, 'headers is not an object', problems);
  }
  if (routing.send) {
    expect(['body', 'query'].includes(routing.send.type), `${path}.send.type`, `unknown send type ${routing.send.type}`, problems);
    expect(typeof routing.send.property === 'string', `${path}.send.property`, 'property is not a string', problems);
  }
  if (routing.output?.postReceive) {
    expect(Array.isArray(routing.output.postReceive), `${path}.output.postReceive`, 'postReceive is not a list', problems);
  }
}

function checkProperty(property, path, problems) {
  expect(typeof property.displayName === 'string' && property.displayName.length > 0, path, 'no displayName', problems);
  expect(typeof property.name === 'string' && /^[a-zA-Z][\w]*$/.test(property.name), path, `name ${JSON.stringify(property.name)} is not an identifier`, problems);
  expect(PROPERTY_TYPES.has(property.type), path, `unknown type ${property.type}`, problems);
  expect('default' in property, path, 'no default', problems);
  if (property.displayOptions) {
    const show = property.displayOptions.show ?? {};
    for (const [key, values] of Object.entries(show)) {
      expect(Array.isArray(values) && values.length > 0, `${path}.displayOptions.show.${key}`, 'is not a non-empty list', problems);
    }
  }
  if (property.type === 'options' || property.type === 'multiOptions') {
    expect(Array.isArray(property.options) && property.options.length > 0, path, 'options without a list', problems);
    for (const [i, option] of (property.options ?? []).entries()) {
      expect(typeof option.name === 'string' && option.name.length > 0, `${path}.options[${i}]`, 'no name', problems);
      expect((typeof option.value === 'string' && option.value.length > 0) || typeof option.value === 'number', `${path}.options[${i}]`, 'no value', problems);
      checkRouting(option.routing, `${path}.options[${i}].routing`, problems);
    }
    if (property.type === 'options') {
      expect((property.options ?? []).some((option) => option.value === property.default), path, 'the default is not among the options', problems);
    }
  }
  if (property.type === 'collection') {
    expect(Array.isArray(property.options), path, 'collection without options', problems);
    for (const [i, option] of (property.options ?? []).entries()) checkProperty(option, `${path}.options[${i}]`, problems);
  }
  checkRouting(property.routing, `${path}.routing`, problems);
}

/** Every problem with a node description; empty when it has the shape n8n loads. */
export function describeProblems(description) {
  const problems = [];
  const path = 'description';
  for (const key of ['displayName', 'name', 'description']) {
    expect(typeof description[key] === 'string' && description[key].length > 0, `${path}.${key}`, 'missing', problems);
  }
  expect(Array.isArray(description.group) && description.group.length > 0, `${path}.group`, 'missing', problems);
  expect(typeof description.version === 'number', `${path}.version`, 'missing', problems);
  expect(typeof description.defaults?.name === 'string', `${path}.defaults.name`, 'missing', problems);
  expect(Array.isArray(description.inputs), `${path}.inputs`, 'missing', problems);
  expect(Array.isArray(description.outputs), `${path}.outputs`, 'missing', problems);
  expect(Array.isArray(description.credentials) && description.credentials.every((c) => typeof c.name === 'string'), `${path}.credentials`, 'missing or unnamed', problems);
  if (description.requestDefaults) {
    expect(typeof description.requestDefaults.baseURL === 'string', `${path}.requestDefaults.baseURL`, 'missing', problems);
  }
  expect(Array.isArray(description.properties) && description.properties.length > 0, `${path}.properties`, 'missing', problems);
  const names = new Map();
  for (const [i, property] of (description.properties ?? []).entries()) {
    checkProperty(property, `${path}.properties[${i}]`, problems);
    // Two properties of one name are fine when they show under different resource/operation
    // pairs - that is how n8n scopes a field - but two under the same pair shadow each other.
    const scope = JSON.stringify(property.displayOptions ?? null);
    const key = `${property.name}@${scope}`;
    expect(!names.has(key), `${path}.properties[${i}]`, `duplicates ${property.name} under the same displayOptions`, problems);
    names.set(key, i);
  }
  return problems;
}

/** Every problem with a credential description. */
export function credentialProblems(credential) {
  const problems = [];
  expect(typeof credential.name === 'string' && /^[a-z][A-Za-z]*$/.test(credential.name), 'credential.name', 'not a camelCase identifier', problems);
  expect(typeof credential.displayName === 'string', 'credential.displayName', 'missing', problems);
  expect(Array.isArray(credential.properties) && credential.properties.length > 0, 'credential.properties', 'missing', problems);
  for (const [i, property] of (credential.properties ?? []).entries()) checkProperty(property, `credential.properties[${i}]`, problems);
  expect(credential.authenticate?.type === 'generic', 'credential.authenticate', 'not generic', problems);
  expect(typeof credential.test?.request?.url === 'string', 'credential.test.request.url', 'missing', problems);
  return problems;
}

/** The selftest habit: a validator that cannot fail proves nothing by passing. */
export function selftest() {
  const good = {
    displayName: 'X', name: 'x', description: 'd', group: ['transform'], version: 1, defaults: { name: 'X' }, inputs: ['main'], outputs: ['main'],
    credentials: [{ name: 'xApi', required: true }], requestDefaults: { baseURL: '={{$credentials.baseUrl}}' },
    properties: [
      { displayName: 'Resource', name: 'resource', type: 'options', default: 'a', options: [{ name: 'A', value: 'a' }] },
      { displayName: 'Operation', name: 'operation', type: 'options', default: 'get', displayOptions: { show: { resource: ['a'] } }, options: [{ name: 'Get', value: 'get', routing: { request: { method: 'GET', url: '/a' } } }] },
    ],
  };
  if (describeProblems(good).length !== 0) return false;
  const broken = [
    (d) => { d.properties[1].options[0].routing.request.method = 'FETCH'; },
    (d) => { delete d.properties[0].default; },
    (d) => { d.properties[0].default = 'zzz'; },
    (d) => { d.properties.push({ ...d.properties[0] }); },
    (d) => { d.properties[0].name = 'not an identifier'; },
    (d) => { delete d.credentials; },
  ];
  return broken.every((mutate) => {
    const copy = structuredClone(good);
    mutate(copy);
    return describeProblems(copy).length > 0;
  });
}
