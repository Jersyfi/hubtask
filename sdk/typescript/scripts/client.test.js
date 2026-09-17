// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The generated TypeScript client against a server that records what it received (P-03). The
// typing is proved by `pnpm typecheck` against dist/schema.d.ts; this proves the runtime half -
// the path filled, the query appended, the bearer and the headers sent, the body encoded, and a
// refusal decoded into ProblemError.

import assert from 'node:assert/strict';
import http from 'node:http';
import test from 'node:test';

import { HubtaskClient, ProblemError } from '../src/client.gen.ts';

async function withServer(handler, run) {
  const received = [];
  const server = http.createServer((request, response) => {
    let body = '';
    request.on('data', (chunk) => (body += chunk));
    request.on('end', () => {
      received.push({ method: request.method, url: request.url, headers: request.headers, body });
      handler(request, response, body);
    });
  });
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const { port } = server.address();
  try {
    await run(`http://127.0.0.1:${port}/api/v1`, received);
  } finally {
    server.close();
  }
}

test('a read fills the path, appends the query and sends the bearer', async () => {
  await withServer(
    (_, response) => {
      response.setHeader('Content-Type', 'application/json');
      response.end(JSON.stringify({ data: [{ id: 'c1', name: 'Private' }], page: { next_cursor: null, has_more: false } }));
    },
    async (baseUrl, received) => {
      const client = new HubtaskClient({ baseUrl, token: () => 'hbt_pat_x', headers: { 'User-Agent': 'test' } });
      const page = await client.listContainers({ query: { type: 'COLLECTION', parent_id: 'h 1', size: 5 } });
      assert.equal(page.data[0].name, 'Private');
      const [call] = received;
      assert.equal(call.method, 'GET');
      assert.equal(call.url, '/api/v1/containers?type=COLLECTION&parent_id=h+1&size=5');
      assert.equal(call.headers.authorization, 'Bearer hbt_pat_x');
      assert.equal(call.headers['user-agent'], 'test');
      assert.match(call.headers.accept, /application\/json/);
    },
  );
});

test('a mutation encodes the body and sends the idempotency key; a path parameter is escaped', async () => {
  await withServer(
    (_, response) => {
      response.statusCode = 201;
      response.setHeader('Content-Type', 'application/json');
      response.end(JSON.stringify({ id: 'i1', title: 'Buy oat milk', version: 1 }));
    },
    async (baseUrl, received) => {
      const client = new HubtaskClient({ baseUrl, token: 'hbt_pat_x' });
      const created = await client.createWorkItem({ type: 'TASK', title: 'Buy oat milk' }, { idempotencyKey: 'k-1' });
      assert.equal(created.id, 'i1');
      await client.updateWorkItem('a/b', { title: 'Renamed' }, { ifMatch: '"1"' });
      const [create, update] = received;
      assert.equal(create.headers['content-type'], 'application/json');
      assert.equal(create.headers['idempotency-key'], 'k-1');
      assert.deepEqual(JSON.parse(create.body), { type: 'TASK', title: 'Buy oat milk' });
      assert.equal(update.method, 'PATCH');
      assert.equal(update.url, '/api/v1/items/a%2Fb');
      assert.equal(update.headers['content-type'], 'application/merge-patch+json');
      assert.equal(update.headers['if-match'], '"1"');
    },
  );
});

test('a refusal is a ProblemError with the document, and a body that is not one still has the status', async () => {
  await withServer(
    (request, response) => {
      if (request.url.endsWith('/meta/health')) {
        response.statusCode = 502;
        response.end('<html>upstream</html>');
        return;
      }
      response.statusCode = 422;
      response.setHeader('Content-Type', 'application/problem+json');
      response.end(JSON.stringify({ status: 422, code: 'validation_failed', detail_code: 'items.title_too_long', field_errors: [{ path: '/title' }] }));
    },
    async (baseUrl) => {
      const client = new HubtaskClient({ baseUrl });
      await assert.rejects(client.createWorkItem({ type: 'TASK', title: '' }), (error) => {
        assert.ok(error instanceof ProblemError);
        assert.equal(error.status, 422);
        assert.equal(error.problem.code, 'validation_failed');
        assert.equal(error.problem.field_errors[0].path, '/title');
        assert.equal(error.message, 'hubtask: 422 validation_failed (items.title_too_long)');
        return true;
      });
      await assert.rejects(client.getHealthReport(), (error) => {
        assert.ok(error instanceof ProblemError);
        assert.equal(error.status, 502);
        assert.equal(error.problem.code, '');
        return true;
      });
    },
  );
});

test('a 204 answers nothing and a raw answer is the response itself', async () => {
  await withServer(
    (request, response) => {
      if (request.method === 'DELETE') {
        response.statusCode = 204;
        response.end();
        return;
      }
      response.setHeader('Content-Type', 'text/calendar');
      response.end('BEGIN:VCALENDAR');
    },
    async (baseUrl, received) => {
      const client = new HubtaskClient({ baseUrl, token: 'hbt_pat_x' });
      assert.equal(await client.trashWorkItem('i1', { ifMatch: '"2"' }), undefined);
      const feed = await client.getCalendarFeedDocument('tok');
      assert.equal(await feed.text(), 'BEGIN:VCALENDAR');
      assert.doesNotMatch(received[1].headers.accept ?? '', /application\/json/, 'a raw answer does not ask for JSON');
    },
  );
});

test('the quickstart example walks a hub against a recording server', async () => {
  const { run } = await import('../examples/quickstart.mjs');
  await withServer(
    (request, response) => {
      response.setHeader('Content-Type', 'application/json');
      if (request.url.startsWith('/api/v1/containers')) {
        response.end(JSON.stringify({ data: [{ id: 'c1', name: 'Inbox' }], page: { next_cursor: null, has_more: false } }));
      } else if (request.method === 'POST') {
        response.statusCode = 201;
        response.end(JSON.stringify({ id: 'i1', title: 'Made by the TypeScript SDK', version: 1 }));
      } else {
        response.end(JSON.stringify({ id: 'i1', title: 'Made by the TypeScript SDK', version: 1 }));
      }
    },
    async (baseUrl, received) => {
      const lines = [];
      const read = await run({ baseUrl, token: 'hbt_pat_x', hub: 'h1', log: (line) => lines.push(line) });
      assert.equal(read.title, 'Made by the TypeScript SDK');
      assert.deepEqual(received.map((call) => `${call.method} ${call.url}`), [
        'GET /api/v1/containers?type=COLLECTION&parent_id=h1',
        'POST /api/v1/items',
        'GET /api/v1/items/i1',
      ]);
      assert.equal(lines.length, 3);
    },
  );
});
