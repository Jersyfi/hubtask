// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// An installed app's passkey step-up through the system browser (ADR-0096 decision 5, UC-SYN-07
// check 3): the workspace's page at its own host runs the ceremony with the options the app put in
// the fragment, and hands the answer to the app's loopback address - with Chromium's virtual
// authenticator holding a real passkey for `localhost`, and the app's listener stood in for by a
// route.
//
// Chromium only: the virtual authenticator is a DevTools protocol of Chromium's own. Served at
// `localhost`, because a browser makes no ceremony for an address.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { generateKeyPairSync, randomBytes } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { answerOf, relayUrl } from '../src/lib/signin/keyRelay.ts';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());
const origin = served.origin.replace('127.0.0.1', 'localhost');

const CREDENTIAL = randomBytes(16);
const CHALLENGE = Buffer.alloc(32, 0x5a).toString('base64url');
const PORT = 51734;
const STATE = 'state-of-the-app-dialog';
const OPTIONS = {
  public_key: {
    rpId: 'localhost', challenge: CHALLENGE, timeout: 300000, userVerification: 'preferred',
    allowCredentials: [{ type: 'public-key', id: CREDENTIAL.toString('base64url') }],
  },
  expires_at: '2099-01-01T00:00:00Z',
};

async function open(browser, address) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const api = [];
  await context.route('**/api/v1/**', (route) => {
    api.push(route.request().url() + (route.request().postData() ?? ''));
    return route.fulfill({ status: 401, json: { code: 'unauthorized', status: 401 } });
  });
  // The app's loopback listener: what the browser was sent there is the whole answer.
  const returned = [];
  await context.route(`http://127.0.0.1:${PORT}/**`, (route) => {
    returned.push(new URL(route.request().url()));
    return route.fulfill({ status: 200, contentType: 'text/html', body: '<h1>Back to the app</h1>' });
  });
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(String(error)));
  const cdp = await context.newCDPSession(page);
  await cdp.send('WebAuthn.enable');
  const { authenticatorId } = await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: {
      protocol: 'ctap2', transport: 'internal', hasResidentKey: true,
      hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true,
    },
  });
  const { privateKey } = generateKeyPairSync('ec', { namedCurve: 'P-256' });
  await cdp.send('WebAuthn.addCredential', {
    authenticatorId,
    credential: {
      credentialId: CREDENTIAL.toString('base64'), isResidentCredential: true, rpId: 'localhost',
      privateKey: privateKey.export({ format: 'der', type: 'pkcs8' }).toString('base64'),
      userHandle: Buffer.alloc(32, 0x33).toString('base64'), signCount: 3,
    },
  });
  await page.goto(address);
  return { context, page, api, returned, failures };
}

test('chromium: the browser answers the app at its loopback address, and tells the server nothing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, api, returned, failures } = await open(browser, relayUrl(origin, { port: PORT, state: STATE, options: OPTIONS }));
  await page.waitForURL(`http://127.0.0.1:${PORT}/**`);

  assert.equal(returned.length, 1);
  const back = returned[0];
  assert.equal(back.pathname, '/oauth');
  assert.equal(back.searchParams.get('state'), STATE);
  const answer = answerOf(back.searchParams.get('code') ?? undefined);
  assert.ok(answer, 'the code is not a key answer');
  assert.equal(answer.raw_id, CREDENTIAL.toString('base64url'));
  const clientData = JSON.parse(Buffer.from(answer.client_data_json, 'base64url').toString('utf8'));
  assert.equal(clientData.type, 'webauthn.get');
  assert.equal(clientData.challenge, CHALLENGE, 'the ceremony answered another challenge than the app was given');
  assert.equal(clientData.origin, origin, 'the passkey did not answer at the workspace host');
  // The options and the answer travel between the app and this page only.
  assert.equal(api.filter((sent) => sent.includes(CHALLENGE) || sent.includes(answer.signature)).length, 0);
  assert.deepEqual(failures, []);
});

test('chromium: the options leave the address at once, and giving up tells the app nothing came', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  t.after(() => context.close());
  const returned = [];
  await context.route('**/api/v1/**', (route) => route.fulfill({ status: 401, json: { code: 'unauthorized', status: 401 } }));
  await context.route(`http://127.0.0.1:${PORT}/**`, (route) => {
    returned.push(new URL(route.request().url()));
    return route.fulfill({ status: 200, contentType: 'text/html', body: '<h1>Back to the app</h1>' });
  });
  // No authenticator: the ceremony is refused, as a person closing the prompt does.
  const page = await context.newPage();
  await page.goto(relayUrl(origin, { port: PORT, state: STATE, options: OPTIONS }));
  const decline = page.getByRole('button', { name: 'Back to the app without it' });
  await decline.waitFor();
  assert.equal(new URL(page.url()).hash, '', 'the options stayed in the address');
  await decline.click();
  await page.waitForURL(`http://127.0.0.1:${PORT}/**`);
  assert.equal(returned[0]?.searchParams.get('error'), 'access_denied');
  assert.equal(returned[0]?.searchParams.get('code'), null);
});

test('chromium: an address that is not the app request asks for nothing', async (t) => {
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const { page, returned } = await open(browser, `${origin}/app/passkey#port=80&state=x`);
  await page.getByText('Nothing is waiting for a passkey here.').waitFor();
  assert.equal(await page.getByRole('button', { name: 'Use a passkey or security key' }).count(), 0);
  assert.equal(returned.length, 0);
});
