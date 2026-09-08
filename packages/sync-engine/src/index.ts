// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What `@hubtask/sync-engine` is.
//
// One entry point, so that a consumer imports the seam rather than a file inside it - and so that
// the day the queue lands behind `SyncEngine`, nothing outside this package has a path to change.

export {
  SyncEngine,
  DEFAULT_TIMEOUT_MS,
  DEFAULT_CONNECT_TIMEOUT_MS,
  DEFAULT_IDLE_TIMEOUT_MS,
  RECONNECT_BASE_MS,
  RECONNECT_MAX_MS,
} from './SyncEngine.ts';
export type {
  Listener,
  ListenOptions,
  MutateOptions,
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
export type {
  ByteTransfer,
  Clock,
  RequestOptions,
  Response,
  Storage,
  StreamConnection,
  StreamEvent,
  StreamOptions,
  Transport,
} from './ports.ts';

export type {
  Account,
  AccountSummary,
  ActivityEntry,
  ActivityPage,
  Assignment,
  AutoAssignOutcome,
  AutoAssignStrategy,
  Bucket,
  Capabilities,
  ChangeRecord,
  Collection,
  Comment,
  CommentPage,
  Container,
  ContainerPage,
  Cover,
  CoverInput,
  DroppedReference,
  FilterNode,
  Group,
  GroupDetail,
  GroupPage,
  HealthReport,
  ItemQueryResult,
  ItemSearchQuery,
  ItemMembers,
  Label,
  Membership,
  MembershipGrant,
  MembershipPage,
  MembershipRole,
  MembershipScope,
  MediaObject,
  MediaPage,
  MediaTransfer,
  MediaUploadRequest,
  MoveResult,
  PendingMutation,
  Problem,
  PurgeSummary,
  QueryField,
  RetentionPolicy,
  StoredRecord,
  SyncCursor,
  TrashEntry,
  TrashPage,
  WorkItem,
  WorkItemPage,
} from './schema.ts';
