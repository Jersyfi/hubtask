// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The shape of a Zapier app definition, as the platform's CLI documents it, as a validator
// (ADR-0058, option A): an app with a version and a platform version, an authentication, and
// triggers, creates and searches each with a key, a noun, a display and an operation - and a
// sample on every trigger and create, which the marketplace review refuses without. What this
// cannot catch is a field the platform renamed; `zapier validate` at publication does.

const FIELD_TYPES = new Set(['string', 'text', 'integer', 'number', 'boolean', 'datetime', 'file', 'password', 'copy']);

function expect(condition, path, message, problems) {
  if (!condition) problems.push(`${path}: ${message}`);
}

function checkFields(fields, path, problems) {
  expect(Array.isArray(fields), path, 'inputFields is not a list', problems);
  const keys = new Set();
  for (const [i, field] of (fields ?? []).entries()) {
    expect(typeof field.key === 'string' && /^[a-zA-Z][\w]*$/.test(field.key), `${path}[${i}]`, `key ${JSON.stringify(field.key)} is not an identifier`, problems);
    expect(!keys.has(field.key), `${path}[${i}]`, `duplicate key ${field.key}`, problems);
    keys.add(field.key);
    if (field.type !== undefined) expect(FIELD_TYPES.has(field.type), `${path}[${i}]`, `unknown type ${field.type}`, problems);
    if (field.choices !== undefined) expect(Array.isArray(field.choices) && field.choices.length > 0, `${path}[${i}]`, 'choices is empty', problems);
    expect(typeof field.label === 'string' && field.label.length > 0, `${path}[${i}]`, 'no label', problems);
  }
}

function checkEntry(entry, path, kind, problems) {
  expect(typeof entry.key === 'string' && /^[a-zA-Z][\w]*$/.test(entry.key), `${path}.key`, 'not an identifier', problems);
  expect(typeof entry.noun === 'string' && entry.noun.length > 0, `${path}.noun`, 'missing', problems);
  expect(typeof entry.display?.label === 'string' && entry.display.label.length > 0, `${path}.display.label`, 'missing', problems);
  expect(typeof entry.display?.description === 'string' && entry.display.description.length > 0, `${path}.display.description`, 'missing', problems);
  const operation = entry.operation ?? {};
  expect(typeof operation.perform === 'function', `${path}.operation.perform`, 'not a function', problems);
  checkFields(operation.inputFields ?? [], `${path}.operation.inputFields`, problems);
  if (kind === 'trigger') {
    expect(operation.type === 'hook' || operation.type === 'polling', `${path}.operation.type`, 'neither hook nor polling', problems);
    if (operation.type === 'hook') {
      for (const name of ['performSubscribe', 'performUnsubscribe', 'performList']) {
        expect(typeof operation[name] === 'function', `${path}.operation.${name}`, 'not a function', problems);
      }
    }
  }
  if (kind !== 'search') {
    expect(typeof operation.sample === 'object' && operation.sample !== null && Object.keys(operation.sample).length > 0, `${path}.operation.sample`, 'missing - the marketplace review refuses an entry without one', problems);
  }
}

/** Every problem with an app definition; empty when it has the shape the platform loads. */
export function appProblems(app) {
  const problems = [];
  expect(typeof app.version === 'string' && /^\d+\.\d+\.\d+$/.test(app.version), 'app.version', 'not a semver', problems);
  expect(typeof app.platformVersion === 'string', 'app.platformVersion', 'missing', problems);
  expect(app.authentication?.type === 'oauth2', 'app.authentication.type', 'not oauth2', problems);
  const oauth = app.authentication?.oauth2Config ?? {};
  for (const name of ['authorizeUrl', 'getAccessToken', 'refreshAccessToken']) {
    expect(oauth[name] !== undefined, `app.authentication.oauth2Config.${name}`, 'missing', problems);
  }
  expect(oauth.enablePkce === true, 'app.authentication.oauth2Config.enablePkce', 'PKCE is required (api-guidelines.md §11)', problems);
  expect(typeof app.authentication?.test === 'object' || typeof app.authentication?.test === 'function', 'app.authentication.test', 'missing', problems);
  for (const [kind, plural] of [['trigger', 'triggers'], ['create', 'creates'], ['search', 'searches']]) {
    expect(typeof app[plural] === 'object' && app[plural] !== null, `app.${plural}`, 'missing', problems);
    for (const [key, entry] of Object.entries(app[plural] ?? {})) {
      expect(entry.key === key, `app.${plural}.${key}`, 'the key and the entry disagree', problems);
      checkEntry(entry, `app.${plural}.${key}`, kind, problems);
    }
  }
  expect(Object.keys(app.triggers ?? {}).length > 0, 'app.triggers', 'empty', problems);
  expect(Object.keys(app.creates ?? {}).length > 0, 'app.creates', 'empty', problems);
  return problems;
}

export function selftest() {
  const good = {
    version: '0.1.0',
    platformVersion: '17.0.0',
    authentication: { type: 'oauth2', oauth2Config: { authorizeUrl: {}, getAccessToken: {}, refreshAccessToken: {}, enablePkce: true }, test: {} },
    triggers: { newItem: { key: 'newItem', noun: 'Item', display: { label: 'New Item', description: 'd' }, operation: { type: 'hook', perform: () => {}, performSubscribe: () => {}, performUnsubscribe: () => {}, performList: () => {}, inputFields: [], sample: { id: 'x' } } } },
    creates: { createItem: { key: 'createItem', noun: 'Item', display: { label: 'Create Item', description: 'd' }, operation: { perform: () => {}, inputFields: [{ key: 'title', label: 'Title', type: 'string', required: true }], sample: { id: 'x' } } } },
    searches: {},
  };
  if (appProblems(good).length !== 0) return false;
  const broken = [
    (a) => { a.authentication.oauth2Config.enablePkce = false; },
    (a) => { delete a.creates.createItem.operation.sample; },
    (a) => { a.creates.createItem.operation.inputFields.push({ key: 'title', label: 'Again' }); },
    (a) => { delete a.triggers.newItem.operation.performUnsubscribe; },
    (a) => { a.creates.createItem.key = 'other'; },
    (a) => { a.creates.createItem.operation.inputFields[0].type = 'colour'; },
  ];
  return broken.every((mutate) => {
    const copy = { ...good, authentication: structuredClone(good.authentication), triggers: { newItem: { ...good.triggers.newItem, operation: { ...good.triggers.newItem.operation } } }, creates: { createItem: { ...good.creates.createItem, operation: { ...good.creates.createItem.operation, inputFields: [...good.creates.createItem.operation.inputFields.map((f) => ({ ...f }))] } } }, searches: {} };
    mutate(copy);
    return appProblems(copy).length > 0;
  });
}
