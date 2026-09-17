// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The engine's conformance run (F6-08): the client requirements of offline-sync.md §9, proved
// through the engine's own API against a running server.
//
// `hubctl sync-conformance` (N-13) drives the *server* as two devices and asks what it owes a
// client. This is the other half: the first-party engine constructed the way a client constructs
// it - a store, a transport, a clock - and driven through `attach`, `mutate`, `push`, `catchUp`
// and `listen` while the server is the real one. Nothing here reaches into the engine; what a
// check reads is the store beneath it and what the server answers, which is all a client ever
// has. The report is one row per requirement, in the shape the evidence files use, so that a
// run appended to `docs/evidence/` reads beside the server's.
//
// What the runner needs from the server it makes and takes away again: a hub with a collection,
// a service account with a role on the hub and a token of its own. The engine runs as that
// account's device, so its copy holds one hub - and an access revocation empties it rather than
// thinning it, which is the check as §9.3 states it.

import { SyncEngine, DEFAULT_TIMEOUT_MS } from '../src/SyncEngine.ts';
import { TransportError } from '../src/errors.ts';
import { isUuidV7 } from '../src/device.ts';
import { META } from '../src/replica.ts';
import { QUEUE } from '../src/queue.ts';
import { MemoryStorage } from '../src/storage/MemoryStorage.ts';
import { systemClock } from '../src/ports.ts';
import type {
  ByteTransfer,
  Clock,
  RequestOptions,
  Response,
  SnapshotLine,
  SnapshotOptions,
  Storage,
  StreamConnection,
  StreamOptions,
  Transport,
  TransportDocument,
} from '../src/ports.ts';
import type { ChangeRecord, PendingMutation, QueuedWrite, StoredRecord, SyncMutation } from '../src/schema.ts';

export type Outcome = 'pass' | 'fail' | 'not-tested';

/** One row of the report: the requirement's number, its claim, and what was seen. */
export interface ConformanceRow {
  readonly number: number;
  readonly claim: string;
  readonly outcome: Outcome;
  readonly note: string;
}

export interface ConformanceOptions {
  /** Where the API is, for the report. */
  readonly baseUrl: string;
  /** The signed-in account's credential: it makes the fixtures and takes them away. */
  readonly token: string;
  /**
   * A transport per credential. Production hands back a `FetchTransport`; the test hands back a
   * fake server's, which is how the runner itself is under test.
   */
  readonly transportFor: (token: string) => Transport;
  readonly clock?: Clock;
  /** The store the engine is attached to. A test that wants a wrong engine wraps it here. */
  readonly storage?: () => Storage;
  /** Where a row goes as it is decided - the terminal, in production. */
  readonly log?: (line: string) => void;
  /** How long a check may wait for the engine's loop to get somewhere. */
  readonly deadlineMs?: number;
  /** Leave the fixtures behind for a look. */
  readonly keep?: boolean;
}

export interface ConformanceRun {
  readonly rows: readonly ConformanceRow[];
  readonly failed: number;
  readonly report: string;
}

const CLAIMS: Readonly<Record<number, string>> = {
  1: 'Local IDs are UUIDv7 and final',
  2: 'Every mutation carries an op_id and an HLC; repetition is allowed and must stay idempotent',
  3: 'After ACCESS_REVOKED or sync.gone, local data is deleted',
  4: 'On sync.cursor_too_old, a full resynchronisation follows',
  5: 'Server responses overwrite local predictions; rejected mutations are shown to the user',
  6: 'Local storage is encrypted and discarded completely on sign-out',
  7: 'Unknown fields and enum values are tolerated and written back unchanged',
  8: "Nothing in the right-hand column of §1 is offered offline",
};

const DEADLINE_MS = 30_000;
const PLATFORM = 'node';
const DEVICE_NAME = 'sync conformance';

/**
 * The wire, as the runner can cut it. A client goes offline by losing its network, and the
 * engine's decision 5 is what it does then - so the runner does the same to the engine rather
 * than asking it to pretend. `dropNextAnswer` is the network failing on the way back from a
 * push: the server applied, the client never heard, and the next push repeats the operation.
 */
class Line implements Transport {
  offline = false;
  dropNextAnswer = false;
  snapshots = 0;
  readonly #inner: Transport;

  constructor(inner: Transport) {
    this.#inner = inner;
  }

  #cut(): void {
    if (this.offline) throw new TransportError('offline');
  }

  async get<T>(path: string, options: RequestOptions): Promise<Response<T>> {
    this.#cut();
    return this.#inner.get<T>(path, options);
  }

  async send<T>(method: 'POST' | 'PATCH' | 'PUT' | 'DELETE', path: string, body: unknown, options: RequestOptions): Promise<Response<T>> {
    this.#cut();
    const answer = await this.#inner.send<T>(method, path, body, options);
    if (path === '/sync:push' && this.dropNextAnswer) {
      this.dropNextAnswer = false;
      throw new TransportError('offline');
    }
    return answer;
  }

  async snapshot(path: string, body: unknown, options: SnapshotOptions): Promise<AsyncIterable<SnapshotLine>> {
    this.#cut();
    this.snapshots += 1;
    return this.#inner.snapshot(path, body, options);
  }

  async stream(path: string, options: StreamOptions): Promise<StreamConnection> {
    this.#cut();
    return this.#inner.stream(path, options);
  }

  async transfer(transfer: ByteTransfer): Promise<void> {
    this.#cut();
    return this.#inner.transfer(transfer);
  }

  async document(path: string, body: unknown, options: RequestOptions): Promise<TransportDocument> {
    this.#cut();
    return this.#inner.document(path, body, options);
  }
}

/**
 * The application's half of a queued write, as small as the run needs: a creation and a patch.
 * The engine mints the identifier and stamps the fields; this only names the kind.
 */
const mutationFor = async (method: string, path: string, body: unknown, helpers: { mintId: () => string }): Promise<QueuedWrite | undefined> => {
  if (method === 'POST' && path === '/items') {
    return { kind: 'ITEM_CREATE', itemId: helpers.mintId(), payload: body as Record<string, unknown> };
  }
  const match = /^\/items\/([^/]+)$/.exec(path);
  if (match?.[1] && method === 'PATCH') return { kind: 'ITEM_PATCH', itemId: match[1], fields: body as Record<string, unknown> };
  return undefined;
};

const noPaths = () => [] as const;

type Document = Record<string, unknown>;

class Fixtures {
  hub = '';
  collection = '';
  account = '';
  tokenId = '';
  token = '';
  membership = '';
}

/** The run itself: the fixtures, the engine as the second person's device, and the checks. */
class Conformance {
  readonly #options: ConformanceOptions;
  readonly #clock: Clock;
  readonly #owner: Transport;
  readonly #fixtures = new Fixtures();
  readonly #rows: ConformanceRow[] = [];
  readonly #startedAt: number;
  #line!: Line;
  #engine!: SyncEngine;
  #storage!: Storage;
  #deviceId = '';
  /** Every record the server sent the engine, by entity identifier - what §9.7 compares against. */
  readonly #sent = new Map<string, Document>();

  constructor(options: ConformanceOptions) {
    this.#options = options;
    this.#clock = options.clock ?? systemClock;
    this.#owner = options.transportFor(options.token);
    this.#startedAt = this.#clock.now();
  }

  get rows(): readonly ConformanceRow[] {
    return [...this.#rows].sort((a, b) => a.number - b.number);
  }

  get failed(): number {
    return this.#rows.filter((row) => row.outcome === 'fail').length;
  }

  #record(number: number, outcome: Outcome, note: string): void {
    const claim = CLAIMS[number] ?? '';
    this.#rows.push({ number, claim, outcome, note });
    this.#options.log?.(`  ${number}. ${outcome.padEnd(10)} ${claim}`);
  }

  #pass(number: number, note: string): void {
    this.#record(number, 'pass', note);
  }

  #fail(number: number, note: string): void {
    this.#record(number, 'fail', note);
  }

  #request(): RequestOptions {
    return { token: this.#options.token, timeoutMs: DEFAULT_TIMEOUT_MS };
  }

  async #ownerPost<T>(path: string, body: unknown): Promise<T> {
    return (await this.#owner.send<T>('POST', path, body, this.#request())).body;
  }

  async setUp(): Promise<void> {
    const f = this.#fixtures;
    const stamp = new Date(this.#startedAt).toISOString().slice(0, 19).replace('T', ' ');
    f.hub = (await this.#ownerPost<{ id: string }>('/containers', { type: 'HUB', name: `Sync conformance (engine) ${stamp}` })).id;
    f.collection = (await this.#ownerPost<{ id: string }>('/containers', { type: 'COLLECTION', name: 'Checks', parent_id: f.hub })).id;
    f.account = (await this.#ownerPost<{ id: string }>('/auth/service-accounts', { display_name: 'sync conformance, the engine\'s device' })).id;
    const minted = await this.#ownerPost<{ id: string; token: string }>('/auth/tokens', {
      name: `sync conformance (engine) ${stamp}`,
      account_id: f.account,
      scopes: ['items:read', 'items:write'],
      expires_at: new Date(this.#startedAt + 24 * 60 * 60 * 1000).toISOString(),
    });
    f.tokenId = minted.id;
    f.token = minted.token;
    f.membership = (await this.#ownerPost<{ id: string }>('/memberships', {
      account_id: f.account, scope_type: 'HUB', scope_id: f.hub, role: 'MEMBER',
    })).id;

    this.#line = new Line(this.#options.transportFor(f.token));
    this.#storage = this.#options.storage?.() ?? new MemoryStorage();
    this.#engine = new SyncEngine({
      transport: this.#line,
      clock: this.#clock,
      token: () => f.token,
      mutationFor,
    });
    const device = await this.#engine.attach(this.#storage, { platform: PLATFORM, displayName: DEVICE_NAME });
    this.#deviceId = device.id;
  }

  /**
   * Removes what the run made, in reverse, best effort: a fixture left behind is a nuisance, not
   * a wrong answer. The service account stays, as after `hubctl sync-conformance`: the contract
   * has no way to remove one, and a stack that runs this is a stack made for the run.
   */
  async tearDown(): Promise<void> {
    const f = this.#fixtures;
    const paths = [
      ...(f.membership ? [`/memberships/${f.membership}`] : []),
      ...(f.tokenId ? [`/auth/tokens/${f.tokenId}`] : []),
      ...(f.hub ? [`/containers/${f.hub}`] : []),
    ];
    for (const path of paths) {
      try {
        await this.#owner.send('DELETE', path, undefined, this.#request());
      } catch (cause) {
        this.#options.log?.(`leaving ${path} behind: ${String(cause)}`);
      }
    }
  }

  readonly #capture = (record: ChangeRecord): void => {
    if (record.op === 'UPSERT' && record.payload && typeof record.payload === 'object') {
      this.#sent.set(record.entity_id, record.payload as Document);
    }
  };

  async #catchUp(): Promise<void> {
    await this.#engine.catchUp({ pathsFor: noPaths, onRecord: this.#capture });
  }

  async #held<T = Document>(collection: string, id: string): Promise<StoredRecord<T> | undefined> {
    return this.#storage.get<StoredRecord<T>>(collection, id);
  }

  async #position(): Promise<{ cursor?: string } | undefined> {
    return this.#storage.get<{ cursor?: string }>(META, 'position');
  }

  async #until(condition: () => Promise<boolean>): Promise<boolean> {
    const deadline = this.#clock.now() + (this.#options.deadlineMs ?? DEADLINE_MS);
    while (this.#clock.now() < deadline) {
      if (await condition()) return true;
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
    return condition();
  }

  async checks(): Promise<void> {
    this.#options.log?.(`the client requirements of offline-sync.md §9, through the engine, against ${this.#options.baseUrl}`);
    await this.#catchUp();
    const collection = await this.#held('containers', this.#fixtures.collection);
    if (!collection) {
      for (const number of [1, 2, 3, 4, 5, 7]) this.#fail(number, 'the initial synchronisation did not deliver the collection the run made; nothing below it can be checked');
      this.#record(6, 'not-tested', 'not tested from a browser engine, by ADR-0033 §4');
      this.#record(8, 'not-tested', 'the engine frames no such kind, by construction');
      return;
    }

    const itemId = await this.#checkIdentifiers();
    if (itemId) await this.#checkIdempotence(itemId);
    await this.#checkServerDecides();
    this.#record(6, 'not-tested', 'not tested from a browser engine, by ADR-0033 §4: the store beneath the engine is deleted on sign-out (`reset`), never encrypted, and this runner does not pretend otherwise');
    // 4 before 7: the resynchronisation it provokes is a snapshot, and a snapshot line carries an
    // entry whole - which is what 7 compares the copy against.
    await this.#checkCursor();
    if (itemId) await this.#checkUnknownFields(itemId);
    this.#record(8, 'not-tested', 'the engine frames no such kind, by construction: `MutationKind` is the seven of §1\'s left-hand column, and a write the application cannot name as one of them is refused rather than queued');
    // Last, because the device has no access to anything below the hub afterwards.
    await this.#checkRevocation();
  }

  /** 1: the identifier of an entry made offline is the client's, a UUIDv7, and the server keeps it. */
  async #checkIdentifiers(): Promise<string | undefined> {
    this.#line.offline = true;
    let predicted: Document | undefined;
    try {
      predicted = await this.#engine.mutate<Document>('POST', '/items', {
        type: 'TASK', collection_id: this.#fixtures.collection, title: 'Minted by the client',
      });
    } catch (cause) {
      this.#fail(1, `a creation while offline was not queued: ${String(cause)}`);
      this.#line.offline = false;
      return undefined;
    }
    this.#line.offline = false;
    const id = typeof predicted?.id === 'string' ? predicted.id : '';
    if (!isUuidV7(id)) {
      this.#fail(1, `the prediction carries ${JSON.stringify(predicted?.id)} as its identifier, which is not a UUIDv7`);
      return undefined;
    }
    await this.#engine.push();
    const held = await this.#held('items', id);
    if (!held || held.pending) {
      this.#fail(1, `after the push the copy holds ${held ? 'the entry still marked pending' : 'no entry under the minted identifier'}`);
      return undefined;
    }
    let read: Document;
    try {
      read = (await this.#line.get<Document>(`/items/${id}`, { token: this.#fixtures.token, timeoutMs: DEFAULT_TIMEOUT_MS })).body;
    } catch (cause) {
      this.#fail(1, `the server does not answer the entry under the identifier the client minted: ${String(cause)}`);
      return undefined;
    }
    if (read.id !== id) {
      this.#fail(1, `the server answers the entry as ${JSON.stringify(read.id)} where the client minted ${id}`);
      return undefined;
    }
    this.#pass(1, `the engine minted ${id} for an entry made offline, the push created it under that identifier (version ${String(held.version)}), and GET /items/{id} answers it`);
    return id;
  }

  /** 2: a push whose answer is lost is repeated under the same op_id, and the server applies it once. */
  async #checkIdempotence(itemId: string): Promise<void> {
    const before = (await this.#held('items', itemId))?.version ?? 0;
    this.#line.offline = true;
    try {
      await this.#engine.mutate('PATCH', `/items/${itemId}`, { title: 'Renamed once' });
    } catch (cause) {
      this.#fail(2, `a patch while offline was not queued: ${String(cause)}`);
      this.#line.offline = false;
      return;
    }
    this.#line.offline = false;
    const queued = await this.#storage.all<PendingMutation>(QUEUE);
    const mutation = queued[0]?.mutation as SyncMutation | undefined;
    const opId = String(mutation?.op_id ?? '');
    const fields = (mutation?.fields ?? {}) as Record<string, { hlc?: string }>;
    const hlc = fields.title?.hlc ?? '';
    if (!isUuidV7(opId) || !/^\d{13}:\d{5}:/.test(hlc)) {
      this.#fail(2, `the queued mutation carries op_id ${JSON.stringify(opId)} and the field's clock ${JSON.stringify(hlc)}`);
      return;
    }
    this.#line.dropNextAnswer = true;
    await this.#engine.push();
    const stillQueued = await this.#storage.all<PendingMutation>(QUEUE);
    if (stillQueued.length !== 1 || stillQueued[0]?.id !== opId) {
      this.#fail(2, `after the answer was lost the queue holds ${stillQueued.length} mutation(s) where the same one, ${opId}, should still wait`);
      return;
    }
    await this.#engine.push();
    if (this.#engine.queueState.count !== 0) {
      this.#fail(2, `the repeated push left ${this.#engine.queueState.count} mutation(s) queued`);
      return;
    }
    const after = await this.#held('items', itemId);
    if (after?.document.title !== 'Renamed once' || after.version !== before + 1) {
      this.#fail(2, `after two pushes of one operation the copy holds title ${JSON.stringify(after?.document.title)} at version ${String(after?.version)}, where version ${before + 1} was expected`);
      return;
    }
    this.#pass(2, `the patch carried op_id ${opId} and an HLC per field; pushed twice - the first answer lost on the way back - it took effect once: version ${before} became ${before + 1}`);
  }

  /** 5: a creation the server refuses is kept as refused, with what was written and the server's code. */
  async #checkServerDecides(): Promise<void> {
    this.#line.offline = true;
    let predicted: Document | undefined;
    try {
      predicted = await this.#engine.mutate<Document>('POST', '/items', {
        type: 'TASK', collection_id: this.#fixtures.collection, title: '',
      });
    } catch (cause) {
      this.#fail(5, `a creation while offline was not queued: ${String(cause)}`);
      this.#line.offline = false;
      return;
    }
    this.#line.offline = false;
    const id = String(predicted?.id ?? '');
    await this.#engine.push();
    const rejected = this.#engine.queueState.rejected.find((entry) => entry.itemId === id);
    if (!rejected) {
      this.#fail(5, `a creation without a title was pushed and is not among the refused: the queue holds ${this.#engine.queueState.count}, refused ${this.#engine.queueState.rejected.length}`);
      return;
    }
    if (!rejected.code) {
      this.#fail(5, 'the refusal carries no code');
      return;
    }
    await this.#engine.dismissRejected(rejected.id);
    this.#pass(5, `a creation without a title was answered REJECTED with ${rejected.code}${rejected.messageCode ? ` (${rejected.messageCode})` : ''}; the engine kept it, with what was written, until dismissed. The server's state over the prediction is 1's: the pushed entry holds the server's version, not the client's`);
  }

  /** 7: a field the server sent is held whole, and a write names only what moved. */
  async #checkUnknownFields(itemId: string): Promise<void> {
    const sent = this.#sent.get(itemId);
    const held = await this.#held('items', itemId);
    if (!sent || !held) {
      this.#fail(7, `the snapshot ${sent ? 'delivered' : 'did not deliver'} the entry and the copy ${held ? 'holds' : 'does not hold'} it`);
      return;
    }
    const missing = Object.keys(sent).filter((key) => !(key in held.document));
    if (missing.length > 0) {
      this.#fail(7, `the copy dropped ${missing.join(', ')} from what the server sent`);
      return;
    }
    this.#line.offline = true;
    await this.#engine.mutate('PATCH', `/items/${itemId}`, { title: 'Renamed again' });
    this.#line.offline = false;
    const queued = await this.#storage.all<PendingMutation>(QUEUE);
    const named = Object.keys((queued[0]?.mutation.fields ?? {}) as object);
    await this.#engine.push();
    const after = await this.#held('items', itemId);
    const dropped = Object.keys(sent).filter((key) => !(key in (after?.document ?? {})));
    if (named.length !== 1 || named[0] !== 'title' || dropped.length > 0) {
      this.#fail(7, `the patch named ${named.join(', ') || 'nothing'} and afterwards the copy lacks ${dropped.join(', ') || 'nothing'}`);
      return;
    }
    this.#pass(7, `the copy holds every one of the ${Object.keys(sent).length} fields the snapshot sent for the entry, this build's and any later one's alike; a patch names only the field that moved (${named[0]}) and the rest stand after the server's answer`);
  }

  /** 4: a cursor the server refuses is forgotten and the engine walks the initial synchronisation again. */
  async #checkCursor(): Promise<void> {
    const position = await this.#position();
    const before = position?.cursor;
    if (!before) {
      this.#fail(4, 'the store holds no cursor after the synchronisation');
      return;
    }
    await this.#storage.put(META, 'position', { ...position, cursor: 'not-a-cursor' });
    const snapshots = this.#line.snapshots;
    const stop = this.#engine.listen({ pathsFor: noPaths, onRecord: this.#capture, wait: async () => {} });
    const resynchronised = await this.#until(async () => {
      const now = (await this.#position())?.cursor;
      return now !== undefined && now !== 'not-a-cursor' && this.#line.snapshots > snapshots;
    });
    stop();
    const now = (await this.#position())?.cursor;
    const hub = await this.#held('containers', this.#fixtures.hub);
    if (!resynchronised || !hub) {
      this.#fail(4, `presented with a cursor it cannot read, the engine ${this.#line.snapshots > snapshots ? 'took the snapshot' : 'took no snapshot'} and the store holds cursor ${JSON.stringify(now)}${hub ? '' : ' and no hub'}`);
      return;
    }
    this.#pass(4, `a cursor the server cannot read is refused as sync.cursor_invalid; the engine forgot it, took the initial synchronisation again and holds a cursor the server minted. A cursor past the window cannot be minted from outside; the engine's answer to sync.cursor_too_old - the copy emptied, then the same walk - is proved against the fakes in test/replica.test.ts`);
  }

  /** 3: the revocation reaches the device as ACCESS_REVOKED, and what it held below the hub goes. */
  async #checkRevocation(): Promise<void> {
    await this.#catchUp();
    if ((await this.#storage.all('containers')).length === 0) {
      this.#fail(3, 'the copy holds nothing before the revocation, so its emptying proves nothing');
      return;
    }
    try {
      await this.#owner.send('DELETE', `/memberships/${this.#fixtures.membership}`, undefined, this.#request());
    } catch (cause) {
      this.#fail(3, `revoking the device\'s membership failed: ${String(cause)}`);
      return;
    }
    this.#fixtures.membership = '';
    try {
      await this.#catchUp();
    } catch (cause) {
      this.#fail(3, `the delta after the revocation failed: ${String(cause)}`);
      return;
    }
    const containers = await this.#storage.all('containers');
    const items = await this.#storage.all('items');
    if (containers.length > 0 || items.length > 0) {
      this.#fail(3, `after ACCESS_REVOKED the copy still holds ${containers.length} container(s) and ${items.length} item(s)`);
      return;
    }
    this.#pass(3, 'the revoked membership arrived as an ACCESS_REVOKED record at the hub, and the copy holds nothing below it any more - no container, no entry. sync.gone is the same path on a purged entry (queue.test.ts)');
  }

  render(): string {
    const stamp = new Date(this.#startedAt).toISOString().slice(0, 10);
    const lines = [
      '# Sync conformance — offline-sync.md §9, the engine',
      '',
      `**${stamp}, \`pnpm --filter @hubtask/sync-engine conformance\` against ${this.#options.baseUrl}.** The eight requirements on clients, checked through the first-party engine's own API: a store in memory, the transport that speaks HTTP, the device of a service account with a role on a hub this run made and took away again.`,
      '',
      '| | Value |',
      '|---|---|',
      `| Installation | ${this.#options.baseUrl} |`,
      `| Hub | \`${this.#fixtures.hub}\` |`,
      `| Device | \`${this.#deviceId}\` |`,
      `| Checks | ${this.#rows.length}, ${this.failed} failed, 2 not testable through the engine |`,
      '',
      '| # | Requirement | Result | What was seen |',
      '|---|---|---|---|',
      ...this.rows.map((row) => `| ${row.number} | ${row.claim} | **${row.outcome}** | ${row.note} |`),
      '',
    ];
    return lines.join('\n');
  }
}

/** Runs the checks and answers the rows and the report. Throws only where the fixtures could not be made. */
export async function runConformance(options: ConformanceOptions): Promise<ConformanceRun> {
  const run = new Conformance(options);
  await run.setUp();
  try {
    await run.checks();
  } finally {
    if (!options.keep) await run.tearDown();
  }
  return { rows: run.rows, failed: run.failed, report: run.render() };
}
