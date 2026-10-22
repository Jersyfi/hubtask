// SPDX-License-Identifier: Apache-2.0
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

import { pollDelay, worthRetrying } from './jobs.ts';

/**
 * The six operations of an entry (`ai-first.md` §2, every row but translation and templates).
 *
 * Each is a `POST /items/{id}:<operation>` answered `202` with the job it queued, or - for
 * `duplicates` - `200` with the suggestion or `204` with nothing near. The answer appears under
 * `GET /suggestions`, and the store re-reads that listing on the job watcher's schedule until it
 * does - reading the job beside it, which is where a refusal met after the ask is said.
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
 * empty list so that a caller who forgot the rule still offers nothing (design-system.md §4,
 * CapabilityGate: absence is absence). `semantic_search` off means `duplicates` has no vectors to compare, which the contract
 * answers `204` for; it is listed with the reason rather than omitted, because a person who has
 * seen it elsewhere would otherwise look for it. An entry without comments has no discussion to
 * summarise, and an archived entry is not written to, so nothing that accepts into it is asked.
 */
export function offersFor(
  features: Readonly<Record<string, unknown>> | undefined,
  item: Pick<WorkItem, 'archived_at'>,
  hasComments: boolean,
  choices: boolean | 'reading' = true,
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
    if (operation === 'classify' && !frozen && choices !== true) {
      // The server refuses a classification with nothing to choose from (`ai.nothing_to_choose_from`),
      // reading the same three lists; until they are read, nothing is offered as possible.
      return { operation, disabledCode: choices === 'reading' ? 'app.suggestions.choices_reading' : 'app.suggestions.no_choices' };
    }
    return { operation, disabledCode: frozen };
  });
}

/** The custom field kinds a classification may choose a value for - the closed ones. */
const CLOSED_KINDS: ReadonlySet<string> = new Set(['SELECT', 'MULTI_SELECT', 'BOOL']);

/** One of the three lists a classification chooses from, as the client holds it. */
type Held<T> = { readonly status: string; readonly data?: readonly T[] } | undefined;

/**
 * Whether a classification of this entry has anything to choose from: a label its collection uses,
 * a column of the board it sits on directly, or a closed custom field for its type - the three sets
 * the server reads before it queues one (`Asking.workable`). `reading` while a list is still
 * being read; a list that failed to read is no reason to withhold the question, so the server's
 * answer decides it then.
 */
export function hasChoices(
  item: Pick<WorkItem, 'type'> & { readonly parent_id?: string | null },
  labels: Held<unknown>,
  buckets: Held<unknown>,
  fields: Held<{ readonly kind: string; readonly applies_to?: readonly string[] }>,
): boolean | 'reading' {
  const lists = [labels, item.parent_id ? { status: 'ready', data: [] } : buckets, fields];
  if (lists.some((list) => list?.status === 'failed')) return true;
  if ((labels?.data ?? []).length > 0) return true;
  if (!item.parent_id && (buckets?.data ?? []).length > 0) return true;
  if ((fields?.data ?? []).some((field) => CLOSED_KINDS.has(field.kind) && (field.applies_to ?? []).includes(item.type))) return true;
  return lists.every((list) => list?.status === 'ready') ? false : 'reading';
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
  | {
      readonly shape: 'fields';
      readonly proposals: readonly FieldProposal[];
      /** The titles the material implied, about a jumble entry: accepting creates them under the converted entry. */
      readonly subtasks: readonly string[];
    }
  | {
      readonly shape: 'classification';
      readonly labelIds: readonly string[];
      readonly bucketId?: string;
      readonly customFields: Readonly<Record<string, unknown>>;
    }
  | { readonly shape: 'breakdown'; readonly nodes: readonly ProposedNode[] }
  | { readonly shape: 'duplicates'; readonly neighbours: readonly Neighbour[] }
  /** A template drafted for the collection: a `TemplateInput`, its tree flattened the way a breakdown is. */
  | {
      readonly shape: 'template';
      readonly name: string;
      readonly description?: string;
      readonly rootType: string;
      readonly nodes: readonly ProposedNode[];
      /** The nodes the collection's profile refused, absent from the tree and counted. */
      readonly dropped: number;
    }
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
function currentOf(
  item: Pick<WorkItem, 'title' | 'notes' | 'due_at'> | undefined,
  field: FieldProposal['field'],
): string | undefined {
  if (!item) return undefined;
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
export function shapeOf(
  suggestion: Pick<Suggestion, 'kind' | 'payload' | 'dropped_nodes'>,
  item: Pick<WorkItem, 'title' | 'notes' | 'due_at'> | undefined,
): Shape {
  const payload: Record<string, unknown> = isRecord(suggestion.payload) ? suggestion.payload : {};

  switch (suggestion.kind) {
    case 'DECOMPOSITION': {
      const nodes: ProposedNode[] = [];
      flatten(payload.children, 0, nodes);
      return { shape: 'breakdown', nodes };
    }
    case 'TEMPLATE': {
      // A draft with no name is a draft nothing could create: `CreateTemplate` requires one, and
      // the server stores no answer without it - so the shape is unknown rather than a tree with
      // an empty heading.
      const name = text(payload.name);
      if (name === undefined) return { shape: 'unknown' };
      const nodes: ProposedNode[] = [];
      flatten(payload.nodes, 0, nodes);
      return {
        shape: 'template',
        name,
        description: text(payload.description),
        rootType: typeof payload.root_type === 'string' ? payload.root_type : '',
        nodes,
        // A row written before the count existed carries none, which reads as nothing dropped.
        dropped: typeof suggestion.dropped_nodes === 'number' && suggestion.dropped_nodes > 0 ? suggestion.dropped_nodes : 0,
      };
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
      return { shape: 'fields', proposals, subtasks: strings(payload.subtasks) };
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
    case 'template':
      return 'app.suggestions.kind_template';
    default:
      return 'app.suggestions.kind_unknown';
  }
}

/** The verb accepting is (§7.3): what it does, not "OK". `DUPLICATES` has none, because nothing accepts it. */
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
    case 'template':
      return 'app.suggestions.accept_template';
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
/**
 * The rule draft an ask produced: the newest proposed `RULE` the person's own listing holds that
 * was made after they asked (UC-AUT-10). Only the asker's drafts are listed at all, so a colleague's
 * cannot be taken for theirs; an older draft of their own stays where it was.
 */
export function ruleDraftSince<T extends Pick<Suggestion, 'kind' | 'status' | 'created_at'>>(
  suggestions: readonly T[],
  askedAt: string,
): T | undefined {
  const asked = Date.parse(askedAt);
  return suggestions
    .filter((each) => each.kind === 'RULE' && each.status === 'PROPOSED' && Date.parse(each.created_at) >= asked)
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))[0];
}

/** Which listing a suggestion stands in. */
export const targetOf = (suggestion: Pick<Suggestion, 'target_type'>): Target =>
  suggestion.target_type === 'JUMBLE_ENTRY' || suggestion.target_type === 'CONTAINER' || suggestion.target_type === 'ACCOUNT'
    ? suggestion.target_type
    : 'WORK_ITEM';

export function arrivedSince(suggestions: readonly Pick<Suggestion, 'created_at' | 'status'>[], askedAt: string): boolean {
  const asked = Date.parse(askedAt);
  return suggestions.some(
    (suggestion) => suggestion.status === 'PROPOSED' && Date.parse(suggestion.created_at) >= asked,
  );
}

/**
 * Sends an ask while its pending state stands, and withdraws that state when the ask is refused.
 *
 * The state is put first so the strip says *Suggesting…* the moment the person asks; a refusal of
 * the ask itself - AI unavailable, nothing to work with, a timeout - ends it at once, and the caller
 * says the refusal. Without the withdrawal the pending card would stand until the screen was left.
 */
export async function whilePending<T>(begin: () => void, withdraw: () => void, send: () => Promise<T>): Promise<T> {
  begin();
  try {
    return await send();
  } catch (error) {
    withdraw();
    throw error;
  }
}

/** What a suggestion is about, as the listing filters it. `ACCOUNT` is a draft for the person who asked. */
export type Target = 'WORK_ITEM' | 'JUMBLE_ENTRY' | 'CONTAINER' | 'ACCOUNT';

/** Where one target's standing proposals are listed. One path, so two readers share one read. */
export const suggestionsPath = (targetId: string, target: Target = 'WORK_ITEM') =>
  `/suggestions?target_type=${target}&target_id=${targetId}`;

/**
 * How many reads of the follow add up to about a minute (`pollDelay`: 0.5 s growing to the 15 s
 * ceiling, about 65 s in all). After them the strip says the AI takes longer than usual, and keeps
 * following.
 */
export const FOLLOW_ATTEMPTS = 10;

/** What a reader of the listing looks like - the one method of the engine the follow needs. */
export interface ListingReader {
  refresh<T>(request: { readonly path: string; readonly timeoutMs?: number }): Promise<
    { readonly status: 'ready'; readonly data: T } | { readonly status: string; readonly error?: unknown }
  >;
}

/** A message code and its parameters, as `renderRefusal` renders one. */
export interface Said {
  readonly code: string;
  readonly params?: Readonly<Record<string, string>>;
}

/** A job that ended with a refusal: the code and parameters its sentence is rendered from. */
export interface Refused {
  readonly refused: Said;
}

/** A job that ended without a proposal, and the code that says why - or nothing to say (a cancelled job, an outcome this version does not know). */
export interface Ended {
  readonly ended: Said | undefined;
}

/** What a follow came to. */
export type FollowOutcome = 'arrived' | 'left' | Refused | Ended;

/**
 * The outcomes of a finished question that are said as a calm note rather than as a proposal: the
 * job's `outcome_code` (`ai-first.md` §2). `ai.proposed` is the one that is not - the proposal is.
 */
const NOTED_OUTCOMES: ReadonlySet<string> = new Set([
  'ai.nothing_to_change',
  'ai.nothing_to_work_with',
  'ai.kept_out',
  'ai.processing_restricted',
]);

/** The part of `GET /jobs/{id}` a follow reads. */
interface JobRead {
  readonly status?: string;
  readonly error_code?: string | null;
  readonly error_params?: Readonly<Record<string, string>> | null;
  readonly outcome_code?: string | null;
}

/**
 * Follows an ask until it ends: the job it queued reaches its end, or - where there is no job to
 * read - a proposal made after `askedAt` stands in the listing.
 *
 * An ask answers `202` with the job that will put the question, and the job says how it ended: a
 * refusal met on the way (`FAILED`, `error_code`), or how the question came out (`SUCCEEDED`,
 * `outcome_code`). A proposal is then read from the listing, which the strip shows; any other
 * outcome is a note. Where the job cannot be read - no `job_id`, or a reader refused it - the
 * listing alone is followed, as before the job said anything.
 *
 * There is no ceiling: every job ends (its deadline, its retries, the dead letter), and a person who
 * asked is owed the answer. After `FOLLOW_ATTEMPTS` reads, about a minute, `onSlow` is called once
 * so the strip can say so honestly. `wait` is injected - the store's pauses while the page is hidden
 * (`waitWhileVisible`), a test's does not wait at all - and `isLive` stops the follow of a screen
 * that was left between two reads.
 */
export async function followArrival(
  reader: ListingReader,
  targetId: string,
  askedAt: string,
  wait: (ms: number) => Promise<void>,
  isLive: () => boolean = () => true,
  target: Target = 'WORK_ITEM',
  jobId?: string,
  onSlow?: () => void,
): Promise<FollowOutcome> {
  let jobReadable = Boolean(jobId);
  for (let attempt = 0; ; attempt++) {
    if (attempt === FOLLOW_ATTEMPTS) onSlow?.();
    await wait(pollDelay(attempt));
    if (!isLive()) return 'left';

    if (jobId && jobReadable) {
      const job = await jobOf(reader, jobId);
      if (!isLive()) return 'left';
      if (job === 'unreadable') {
        jobReadable = false;
      } else {
        if (job && isEnd(job.status)) return endOf(reader, targetId, askedAt, target, job);
        continue;
      }
    }

    if (await hasArrived(reader, targetId, askedAt, target)) return isLive() ? 'arrived' : 'left';
    if (!isLive()) return 'left';
  }
}

const isEnd = (status: string | undefined) => status === 'SUCCEEDED' || status === 'FAILED' || status === 'CANCELLED';

/** How a job that ended is said: the refusal, the note, or the proposal the listing now shows. */
async function endOf(
  reader: ListingReader,
  targetId: string,
  askedAt: string,
  target: Target,
  job: JobRead,
): Promise<Exclude<FollowOutcome, 'left'>> {
  if (job.status === 'FAILED') {
    return job.error_code
      ? { refused: { code: job.error_code, params: job.error_params ?? undefined } }
      : { refused: { code: 'app.something_went_wrong' } };
  }
  if (job.status === 'CANCELLED') return { ended: undefined };

  const outcome = job.outcome_code ?? undefined;
  if (outcome && NOTED_OUTCOMES.has(outcome)) return { ended: { code: outcome } };
  // A proposal - or a job that said nothing, from a worker of the release before this one: the
  // listing is read once, so the strip shows what stands.
  const arrived = await hasArrived(reader, targetId, askedAt, target);
  if (arrived || outcome === 'ai.proposed') return 'arrived';
  return outcome ? { ended: undefined } : { ended: { code: 'ai.nothing_to_change' } };
}

/** Whether the listing shows a proposal made after the ask. A read that failed shows nothing. */
async function hasArrived(reader: ListingReader, targetId: string, askedAt: string, target: Target): Promise<boolean> {
  try {
    const read = await reader.refresh<SuggestionPage>({ path: suggestionsPath(targetId, target), timeoutMs: FOLLOW_READ_TIMEOUT_MS });
    return read.status === 'ready' && arrivedSince((read as { data: SuggestionPage }).data.items ?? [], askedAt);
  } catch {
    return false;
  }
}

/**
 * The job as it stands; nothing for a read that failed and is worth repeating (a timeout, a dead
 * network); `unreadable` where the server will not tell this reader about the job (`403`, `404`),
 * which asking again every round would not change.
 */
async function jobOf(reader: ListingReader, jobId: string): Promise<JobRead | 'unreadable' | undefined> {
  try {
    const read = await reader.refresh<JobRead>({ path: `/jobs/${jobId}`, timeoutMs: FOLLOW_READ_TIMEOUT_MS });
    if (read.status === 'ready') return (read as { data: JobRead }).data;
    if (read.status === 'failed' && 'error' in read && !worthRetrying(read.error)) return 'unreadable';
    return undefined;
  } catch (error) {
    return worthRetrying(error) ? undefined : 'unreadable';
  }
}

/** How long one read of the follow may take. */
const FOLLOW_READ_TIMEOUT_MS = 10_000;

/** The part of `document` a paused wait reads. */
export interface Page {
  readonly visibilityState: string;
  addEventListener(type: 'visibilitychange', listener: () => void): void;
  removeEventListener(type: 'visibilitychange', listener: () => void): void;
}

/**
 * A wait of `ms` that does not end while the page is hidden: it ends once the page is shown again.
 *
 * A follow has no ceiling, so a tab left open overnight would otherwise read the job every 15 s
 * for nothing; paused, it reads once more when the person looks again. `page` is `document` in the
 * browser and nothing where there is none, which waits plainly.
 */
export function waitWhileVisible(
  page: Page | undefined,
  timer: (ms: number) => Promise<void> = (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
): (ms: number) => Promise<void> {
  return async (ms) => {
    await timer(ms);
    if (!page || page.visibilityState !== 'hidden') return;
    await new Promise<void>((resolve) => {
      const shown = () => {
        if (page.visibilityState === 'hidden') return;
        page.removeEventListener('visibilitychange', shown);
        resolve();
      };
      page.addEventListener('visibilitychange', shown);
    });
  };
}

/** How an ask stands, as the strip records it: following (and slow), or how it ended. */
export type AskingOutcome = 'following' | 'slow' | 'arrived' | 'ended' | 'nothing_near' | 'refused';

/** The part of an ask's state the strips read to say it. */
export interface AskingState {
  readonly outcome: AskingOutcome;
  /** With `refused`: the refusal the job ended with. */
  readonly refusal?: Said;
  /** With `ended`: why it ended without a proposal, or nothing to say. */
  readonly note?: Said;
}

/** What a strip says about an ask, and whether it is a refusal or a calm note. */
export interface AskingNote extends Said {
  readonly tone: 'note' | 'refusal';
}

/** Whether the ask is still being followed: the pending card stands, and asking again waits. */
export function isPending(asking: AskingState | undefined): boolean {
  return asking?.outcome === 'following' || asking?.outcome === 'slow';
}

/**
 * The one reading of an ask's state for the four screens that ask (an entry, a jumble card, a
 * collection, a template draft): what to say, or nothing - a proposal says itself, and a pending
 * ask says *Suggesting…* on its card until about a minute has gone by.
 */
export function noteOf(asking: AskingState | undefined): AskingNote | undefined {
  switch (asking?.outcome) {
    case 'slow':
      return { code: 'app.suggestions.slow', tone: 'note' };
    case 'refused':
      return asking.refusal ? { ...asking.refusal, tone: 'refusal' } : undefined;
    case 'ended':
      return asking.note ? { ...asking.note, tone: 'note' } : undefined;
    case 'nothing_near':
      return { code: 'app.suggestions.nothing_near', tone: 'note' };
    default:
      return undefined;
  }
}

/**
 * How a proposal prepared on arrival ended, as its card records it (UC-JUM-07 check 10). Nobody
 * asked, so nothing is said on the card but the proposal itself: a spent budget is said once for
 * the page (`budgetSpentOf`), and every other ending - switched off, the day's number reached,
 * nothing to send, AI off - leaves the arrival waiting for somebody to ask, quietly.
 */
export function arrivalEndedAs(outcome: Exclude<FollowOutcome, 'left'>): AskingState {
  return outcome === 'arrived' ? { outcome: 'arrived' } : { outcome: 'ended' };
}

/** The spent budget a follow ended on, where it did: what the jumble says once for the page. */
export function budgetSpentOf(outcome: FollowOutcome): Said | undefined {
  if (typeof outcome === 'object' && 'refused' in outcome && outcome.refused.params?.reason === 'budget_spent') {
    return outcome.refused;
  }
  return undefined;
}

/** What an ask that ended records, from what its follow came to. */
export function endedAs(outcome: Exclude<FollowOutcome, 'left'>): AskingState {
  if (outcome === 'arrived') return { outcome: 'arrived' };
  if ('refused' in outcome) return { outcome: 'refused', refusal: outcome.refused };
  return { outcome: 'ended', note: outcome.ended };
}
