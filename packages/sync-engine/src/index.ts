// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What `@hubtask/sync-engine` is.
//
// One entry point, so that a consumer imports the seam rather than a file inside it - and so that
// the day the queue lands behind `SyncEngine`, nothing outside this package has a path to change.

export { SyncEngine, DEFAULT_TIMEOUT_MS } from './SyncEngine.ts';
export type {
  Listener,
  ResourceRequest,
  ResourceState,
  SyncEngineOptions,
  Unsubscribe,
} from './SyncEngine.ts';

export { FetchTransport } from './FetchTransport.ts';
export type { FetchTransportOptions } from './FetchTransport.ts';

export { TransportError } from './errors.ts';
export type { FailureKind, FieldProblem } from './errors.ts';

export { systemClock } from './ports.ts';
export type { Clock, RequestOptions, Response, Storage, Transport } from './ports.ts';

export type {
  Account,
  Capabilities,
  Collection,
  Container,
  HealthReport,
  PendingMutation,
  Problem,
  StoredRecord,
  SyncCursor,
  WorkItem,
} from './schema.ts';
