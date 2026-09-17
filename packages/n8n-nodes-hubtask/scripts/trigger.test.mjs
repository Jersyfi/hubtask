// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The trigger's webhook half against a delivery as the installation sends one: a CloudEvents
// document under `application/cloudevents+json`, which n8n's body parser leaves unread, signed
// over its bytes (automation.md §3.1, issue 723).

import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import { createRequire } from 'node:module';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

// The node reads events.json beside itself, which the build writes into dist/; the test stands the
// node up in a directory of its own so that it does not depend on a build having run.
function loadTrigger() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'hubtask-trigger-'));
  fs.copyFileSync(path.join(packageRoot, 'nodes', 'HubtaskTrigger', 'HubtaskTrigger.node.js'), path.join(dir, 'HubtaskTrigger.node.js'));
  fs.writeFileSync(path.join(dir, 'events.json'), JSON.stringify(['de.hubtask.work.item.completed.v1']));
  const { HubtaskTrigger } = require(path.join(dir, 'HubtaskTrigger.node.js'));
  return new HubtaskTrigger();
}

const secret = 'whsec_test';
const delivery = JSON.stringify({
  specversion: '1.0',
  type: 'de.hubtask.work.item.completed.v1',
  id: '0192f000-0000-7000-8000-0000000000e1',
  data: { id: '0192f000-0000-7000-8000-00000000000e', title: 'Book the venue' },
});

function signed(body, at = Math.floor(Date.now() / 1000)) {
  const v1 = crypto.createHmac('sha256', secret).update(`${at}.${body}`).digest('hex');
  return `t=${at},v1=${v1}`;
}

/** What n8n hands the node: the bytes on `rawBody`, `body` empty because the parser did not read it. */
function context({ rawBody, body = {}, signature, staticData = { secret }, readRawBody, warnings = [] }) {
  const request = { rawBody, body };
  if (readRawBody) request.readRawBody = readRawBody;
  return {
    getWorkflowStaticData: () => staticData,
    getRequestObject: () => request,
    getHeaderData: () => (signature === undefined ? {} : { 'x-hubtask-signature': signature }),
    helpers: { returnJsonArray: (json) => (Array.isArray(json) ? json : [json]).map((item) => ({ json: item })) },
    logger: { warn: (message) => warnings.push(message) },
  };
}

test('a cloudevents+json delivery is read from the raw bytes, and the item is the event', async () => {
  const trigger = loadTrigger();
  const answer = await trigger.webhook.call(context({ rawBody: Buffer.from(delivery), signature: signed(delivery) }));
  assert.deepEqual(answer, { workflowData: [[{ json: JSON.parse(delivery) }]] });
  assert.equal(answer.workflowData[0][0].json.data.title, 'Book the venue');
});

test('the signature is verified over the bytes, and a forged one is 401', async () => {
  const trigger = loadTrigger();
  const forged = delivery.replace('Book the venue', 'Cancel the venue');
  const answer = await trigger.webhook.call(context({ rawBody: Buffer.from(forged), signature: signed(delivery) }));
  assert.deepEqual(answer, { webhookResponse: { status: 401 } });

  const unsigned = await trigger.webhook.call(context({ rawBody: Buffer.from(delivery) }));
  assert.deepEqual(unsigned, { webhookResponse: { status: 401 } });

  const stale = await trigger.webhook.call(
    context({ rawBody: Buffer.from(delivery), signature: signed(delivery, Math.floor(Date.now() / 1000) - 600) }),
  );
  assert.deepEqual(stale, { webhookResponse: { status: 401 } });
});

test('an n8n that reads the body lazily is asked for it', async () => {
  const trigger = loadTrigger();
  let reads = 0;
  const answer = await trigger.webhook.call(
    context({
      signature: signed(delivery),
      readRawBody: async function () {
        reads++;
        this.rawBody = Buffer.from(delivery);
      },
    }),
  );
  assert.equal(reads, 1);
  assert.equal(answer.workflowData[0][0].json.data.title, 'Book the venue');
});

test('bytes that are not a document are 400, and no secret held is said rather than hidden', async () => {
  const trigger = loadTrigger();
  const broken = await trigger.webhook.call(context({ rawBody: Buffer.from('not json'), signature: signed('not json') }));
  assert.deepEqual(broken, { webhookResponse: { status: 400 } });

  const warnings = [];
  const unverified = await trigger.webhook.call(context({ rawBody: Buffer.from(delivery), staticData: {}, warnings }));
  assert.equal(unverified.workflowData[0][0].json.data.title, 'Book the venue');
  assert.equal(warnings.length, 1);
});
