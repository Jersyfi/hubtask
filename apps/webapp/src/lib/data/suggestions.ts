// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a screen can work out about a suggestion without asking the server again.
 *
 * The pure half of `suggestions.svelte.ts`: which of the six operations an entry may be asked,
 * whether a suggestion is stale, and what shape its payload has. Separate from the store for the
 * reason `jobs.ts` is separate from its watcher — every rule here is a question with an answer a
 * test can assert without a browser or a rune.
 *
 * **A payload is data, never an instruction.** The contract says so of `Suggestion.payload`, and
 * this module reads it the way `live.ts` reads a change record: a shape to render, never a value
 * to act on. Accepting is the server's own use case with the accepting person's rights; nothing
 * here writes a field of the entry.
 */

import type { Suggestion, SuggestionPage, WorkItem } from '@hubtask/sync-engine';

import { pollDelay } from './jobs.ts';

/**
 * The six operations of an entry (`ai-first.md` §2, every row but translation and templates).
 *
 * Each is a `POST /items/{id}:<operation>` answered `202` with no body, or - for `duplicates` -
 * `200` with the suggestion or `204` with nothing near. Nothing here is a job the client can
 * follow: the answer appears under `GET /suggestions`, and the store re-reads that listing on the
 * job watcher's schedule until it does.
 */
export type Operation =
  | 'suggest-fields'
  | 'summarize'
  | 'classify'
  | 'decompose'
  | 'summarize-thread'
  | 'duplicates';

export const OPERATIONS: readonly Operation[] = [
  'suggest-fields',
  'summarize',
  'classify',
  'decompose',
  'summarize-thread',
  'duplicates',
];

/** What the menu needs to know to offer an operation. */
export interface Offer {
  readonly operation: Operation;
  /** Why it cannot be asked right now, or nothing when it can. A message code, rendered by the caller. */
  readonly disabledCode?: string;
}

/**
 * Which operations the installation and the entry allow, with the reason for each that does not.
 *
 * `ai_suggestions` off means no menu at all - the caller renders nothing, and this answers an
 * empty list so that a caller who forgot the rule still offers nothing (decision 4: absence is
 * absence). `semantic_search` off means `duplicates` has no vectors to compare, which the contract
 * answers `204` for; it is listed with the reason rather than omitted, because a person who has
 * seen it elsewhere would otherwise look for it. An entry without comments has no discussion to
 * summarise, and an archived entry is not written to, so nothing that accepts into it is asked.
 */
export function offersFor(
  features: Readonly<Record<string, unknown>> | undefined,
  item: Pick<WorkItem, 'archived_at'>,
  hasComments: boolean,
): readonly Offer[] {
  if (features?.ai_suggestions !== true) return [];
  const frozen = item.archived_at ? 'app.entries.archived' : undefined;
  return OPERATIONS.map((operation) => {
    if (operation === 'duplicates') {
      return {
        operation,
        disabledCode: features.semantic_search === true ? undefined : 'app.suggestions.no_semantic_search',
      };
    }
    if (operation === 'summarize-thread' && !hasComments) {
      return { operation, disabledCode: 'app.suggestions.no_comments' };
    }
    return { operation, disabledCode: frozen };
  });
}

/**
 * Whether the entry has moved since the suggestion was made.
 *
 * `suggestions.stale` is the server's refusal on `:accept`, and this is the same fact read from
 * what the client already holds: a proposal produced before the entry's last change is a proposal
 * about a state the entry is no longer in. Marked here so the strip says so before the server has
 * to - and the server's word still wins where the two disagree, because a refusal it sends is
 * rendered like any other.
 */
export function isStale(suggestion: Pick<Suggestion, 'produced_at'>, item: Pick<WorkItem, 'updated_at'>): boolean {
  const produced = Date.parse(suggestion.produced_at);
  // An older server may answer no `updated_at`; then nothing is known and nothing is marked.
  const changed = Date.parse(item.updated_at ?? '');
  if (Number.isNaN(produced) || Number.isNaN(changed)) return false;
  return produced < changed;
}

/** One proposed value of the entry's own fields, beside what the entry holds today. */
export interface FieldProposal {
  readonly field: 'title' | 'notes' | 'due_date';
  readonly proposed: string;
  /** What the entry holds now, or nothing where it holds nothing. */
  readonly current?: string;
}

/** One proposed node of a breakdown, flattened with its depth for a list to draw. */
export interface ProposedNode {
  readonly depth: number;
  readonly type: string;
  readonly title: string;
  readonly notes?: string;
}

/** One entry that looks like this one. An identifier and a number, which is all the payload carries. */
export interface Neighbour {
  readonly itemId: string;
  readonly similarity: number;
}

/**
 * The shapes a payload takes. The kind says what accepting does; the shape says what to draw -
 * and a `FIELDS` suggestion is three shapes, because a summary, a classification and a field
 * proposal are one kind with three payloads (`SuggestionKind`'s own words).
 */
export type Shape =
  | { readonly shape: 'fields'; readonly proposals: readonly FieldProposal[] }
  | {
      readonly shape: 'classification';
      readonly labelIds: readonly string[];
      readonly bucketId?: string;
      readonly customFields: Readonly<Record<string, unknown>>;
    }
  | { readonly shape: 'breakdown'; readonly nodes: readonly ProposedNode[] }
  | { readonly shape: 'duplicates'; readonly neighbours: readonly Neighbour[] }
  | { readonly shape: 'unknown' };

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

const text = (value: unknown): string | undefined =>
  typeof value === 'string' && value.trim() !== '' ? value : undefined;

const strings = (value: unknown): string[] =>
  Array.isArray(value) ? value.filter((entry): entry is string => typeof entry === 'string') : [];

/**
 * A breakdown as one list, parent before children, each with its depth.
 *
 * The server bounds the tree (`keptTree`), so the recursion is bounded by what was stored; the
 * cap here is defence against a payload nothing here wrote.
 */
function flatten(children: unknown, depth: number, out: ProposedNode[]): void {
  if (!Array.isArray(children) || depth > 8) return;
  for (const node of children) {
    if (!isRecord(node)) continue;
    const title = text(node.title);
    if (title === undefined) continue;
    out.push({
      depth,
      type: typeof node.type === 'string' ? node.type : '',
      title,
      notes: text(node.notes),
    });
    flatten(node.children, depth + 1, out);
  }
}

/** What the entry holds under a field the proposal names, as the strip shows it beside the proposal. */
function currentOf(item: Pick<WorkItem, 'title' | 'notes' | 'due_at'>, field: FieldProposal['field']): string | undefined {
  switch (field) {
    case 'title':
      return item.title;
    case 'notes':
      return text(item.notes);
    case 'due_date':
      return text(item.due_at);
  }
}

/**
 * Reads the payload into the shape the strip renders.
 *
 * A `FIELDS` payload that names a label, a bucket or a custom field is a classification; one that
 * names a title, notes or a date is a field proposal; one that names both is drawn as fields with
 * the classification alongside, which the store keeps out of this function by never asking for
 * both in one prompt (`promptFields` on the server). A kind or a payload this version has never
 * met is `unknown`, and the strip says so rather than drawing braces - the same tolerance every
 * other reader of the contract has.
 */
export function shapeOf(suggestion: Pick<Suggestion, 'kind' | 'payload'>, item: Pick<WorkItem, 'title' | 'notes' | 'due_at'>): Shape {
  const payload: Record<string, unknown> = isRecord(suggestion.payload) ? suggestion.payload : {};

  switch (suggestion.kind) {
    case 'DECOMPOSITION': {
      const nodes: ProposedNode[] = [];
      flatten(payload.children, 0, nodes);
      return { shape: 'breakdown', nodes };
    }
    case 'DUPLICATES': {
      const neighbours = (Array.isArray(payload.duplicates) ? payload.duplicates : [])
        .filter(isRecord)
        .flatMap((entry) => {
          const itemId = text(entry.item_id);
          const similarity = typeof entry.similarity === 'number' ? entry.similarity : undefined;
          return itemId !== undefined && similarity !== undefined ? [{ itemId, similarity }] : [];
        });
      return { shape: 'duplicates', neighbours };
    }
    case 'FIELDS': {
      const labelIds = strings(payload.label_ids);
      const bucketId = text(payload.bucket_id);
      const customFields = isRecord(payload.custom_fields) ? payload.custom_fields : {};
      if (labelIds.length > 0 || bucketId !== undefined || Object.keys(customFields).length > 0) {
        return { shape: 'classification', labelIds, bucketId, customFields };
      }
      const proposals: FieldProposal[] = [];
      for (const field of ['title', 'notes', 'due_date'] as const) {
        const proposed = text(payload[field]);
        if (proposed === undefined) continue;
        proposals.push({ field, proposed, current: currentOf(item, field) });
      }
      return { shape: 'fields', proposals };
    }
    default:
      return { shape: 'unknown' };
  }
}

/** Whether a field proposal is a summary: notes and nothing else, which is what `summarize` answers. */
export function isSummary(shape: Shape): boolean {
  return shape.shape === 'fields' && shape.proposals.length > 0 && shape.proposals.every((p) => p.field === 'notes');
}

/** The heading's code per shape - what the kind is called when it is offered (voice-and-tone.md §7.1). */
export function headingCodeOf(shape: Shape): string {
  switch (shape.shape) {
    case 'fields':
      return isSummary(shape) ? 'app.suggestions.kind_summary' : 'app.suggestions.kind_fields';
    case 'classification':
      return shape.labelIds.length > 0 && shape.bucketId === undefined && Object.keys(shape.customFields).length === 0
        ? 'app.suggestions.kind_labels'
        : 'app.suggestions.kind_classification';
    case 'breakdown':
      return 'app.suggestions.kind_breakdown';
    case 'duplicates':
      return 'app.suggestions.kind_duplicates';
    default:
      return 'app.suggestions.kind_unknown';
  }
}

/** The verb accepting is (§7.3): what it does, not "OK". `DUPLICATES` has none, because nothing accepts it (K-04). */
export function acceptCodeOf(shape: Shape): string | undefined {
  switch (shape.shape) {
    case 'fields':
      return isSummary(shape) ? 'app.suggestions.accept_summary' : 'app.suggestions.accept_fields';
    case 'classification': {
      // The verb names what accepting does, and a classification does one of three things.
      const hasLabels = shape.labelIds.length > 0;
      const hasColumn = shape.bucketId !== undefined;
      const hasFields = Object.keys(shape.customFields).length > 0;
      if (hasLabels && !hasColumn && !hasFields) return 'app.suggestions.accept_labels';
      if (hasColumn && !hasLabels && !hasFields) return 'app.suggestions.accept_column';
      return 'app.suggestions.accept_classification';
    }
    case 'breakdown':
      return 'app.suggestions.accept_breakdown';
    default:
      return undefined;
  }
}

/**
 * The operation a proposal is asked again as, read from the prompt that made it - the one fact
 * that tells a summary of the entry from a summary of its discussion - and from the shape where
 * the prompt is one this version has never met.
 */
export function operationOf(suggestion: Pick<Suggestion, 'prompt_id'>, shape: Shape): Operation {
  switch (suggestion.prompt_id) {
    case 'summarize':
      return 'summarize';
    case 'summarize-thread':
      return 'summarize-thread';
    case 'classify':
      return 'classify';
    case 'decompose':
      return 'decompose';
    case 'suggest-item-fields':
      return 'suggest-fields';
  }
  switch (shape.shape) {
    case 'classification':
      return 'classify';
    case 'breakdown':
      return 'decompose';
    case 'duplicates':
      return 'duplicates';
    default:
      return isSummary(shape) ? 'summarize' : 'suggest-fields';
  }
}

/**
 * Whether the listing shows something the ask is waiting for: a proposal made after the ask.
 *
 * The store re-reads the listing after an ask until this answers true or the schedule runs out.
 * "Made after" rather than "one more than before", because a second reader of the same entry may
 * have asked in the meantime and their answer is as good as ours - the strip shows what stands,
 * not what this tab caused.
 */
export function arrivedSince(suggestions: readonly Pick<Suggestion, 'created_at' | 'status'>[], askedAt: string): boolean {
  const asked = Date.parse(askedAt);
  return suggestions.some(
    (suggestion) => suggestion.status === 'PROPOSED' && Date.parse(suggestion.created_at) >= asked,
  );
}

/** Where one entry's standing proposals are listed. One path, so two readers share one read. */
export const suggestionsPath = (itemId: string) => `/suggestions?target_type=WORK_ITEM&target_id=${itemId}`;

/** How many times the listing is re-read after an ask before the strip stops waiting. */
export const FOLLOW_ATTEMPTS = 10;

/** What a reader of the listing looks like - the one method of the engine the follow needs. */
export interface ListingReader {
  refresh<T>(request: { readonly path: string; readonly timeoutMs?: number }): Promise<
    { readonly status: 'ready'; readonly data: T } | { readonly status: string }
  >;
}

/**
 * Re-reads the listing on the job watcher's schedule until a proposal made after `askedAt`
 * stands in it, or the schedule runs out.
 *
 * An ask answers `202` with no body - the provider has not been asked yet, so there is no job to
 * name - and the change stream carries no suggestion record, so the listing is the one place the
 * answer arrives. `wait` is injected so that a test runs the schedule without the clock; `isLive`
 * lets a screen that was left stop the follow between two reads.
 */
export async function followArrival(
  reader: ListingReader,
  itemId: string,
  askedAt: string,
  wait: (ms: number) => Promise<void>,
  isLive: () => boolean = () => true,
): Promise<'arrived' | 'gave_up' | 'left'> {
  for (let attempt = 0; attempt < FOLLOW_ATTEMPTS; attempt++) {
    await wait(pollDelay(attempt));
    if (!isLive()) return 'left';
    const read = await reader.refresh<SuggestionPage>({ path: suggestionsPath(itemId), timeoutMs: FOLLOW_READ_TIMEOUT_MS });
    if (!isLive()) return 'left';
    if (read.status === 'ready' && arrivedSince((read as { data: SuggestionPage }).data.items ?? [], askedAt)) {
      return 'arrived';
    }
  }
  return 'gave_up';
}

/** How long one re-read of the listing may take. */
const FOLLOW_READ_TIMEOUT_MS = 10_000;
