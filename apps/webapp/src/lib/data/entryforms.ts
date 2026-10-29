// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A type's form as the web app draws and edits it (UC-FLW-01 check 2, D-13). Pure, so each
 * decision is tested; `entrytypes.svelte.ts` sends the form and asks the server how it stands.
 *
 * **The server decides, the client draws its answer.** Capture and the entry panel ask
 * `POST /entry-types/{key}/form:evaluate` with the draft and draw the fields as the answer says -
 * required, read-only, hidden. The write is held to the same answer by the server, so what this
 * screen shows and what the server refuses come from one evaluator. Offline, `offlineFormState`
 * stands in: the browser's evaluator (`@hubtask/expression`) reads the same conditions over the
 * copy and what the device last learnt of the reader's roles, through the door the write will
 * arrive by (`SYNC`), and the server judges the write again once it arrives.
 */

import { evaluateForm, type FormActor } from '@hubtask/expression';
import type { AccessExplanation, EntryForm, EntryFormChannel, EntryFormDraft, EntryFormField, EntryFormState, EntryType, WorkItem } from '@hubtask/sync-engine';

/** The entry's own values a form may show as fields, besides its custom fields. */
export const OWN_FORM_KEYS = ['title', 'notes', 'due_date', 'start_at'] as const;

/** The doors a form tells apart, in the contract's order. */
export const FORM_CHANNELS = ['APP', 'MCP', 'RULE', 'SYNC', 'IMPORT', 'MAIL'] as const;

const CUSTOM = 'custom_fields.';

/** The custom key a form key names, or undefined for one of the entry's own. */
export function customKeyOf(key: string): string | undefined {
  return key.startsWith(CUSTOM) ? key.slice(CUSTOM.length) : undefined;
}

/** The form key of a custom field. */
export const formKeyOfCustom = (key: string): string => `${CUSTOM}${key}`;

/** Whether the form asks anything: what every type has until somebody designs one. */
export function hasForm(form: EntryForm | undefined): form is EntryForm {
  return Boolean(form && ((form.fields?.length ?? 0) > 0 || (form.validations?.length ?? 0) > 0 || (form.channels?.length ?? 0) > 0));
}

/** One heading of the form and its fields, in the form's order; the fields of no section first. */
export interface FormGroup {
  readonly section?: { readonly key: string; readonly name: string };
  readonly fields: readonly EntryFormField[];
}

/** The form's fields grouped by section, empty sections left out. */
export function groupsOf(form: EntryForm | undefined): readonly FormGroup[] {
  const fields = form?.fields ?? [];
  const loose = fields.filter((field) => !field.section);
  const groups: FormGroup[] = loose.length > 0 ? [{ fields: loose }] : [];
  for (const section of form?.sections ?? []) {
    const inside = fields.filter((field) => field.section === section.key);
    if (inside.length > 0) groups.push({ section, fields: inside });
  }
  return groups;
}

/** How one field stands: the server's answer where there is one, the form's own words otherwise. */
export interface FieldStanding {
  readonly required: boolean;
  readonly readOnly: boolean;
  readonly hidden: boolean;
}

export function standingOf(field: EntryFormField, state: EntryFormState | undefined): FieldStanding {
  const answered = state?.fields[field.key];
  if (answered) return { required: answered.required, readOnly: answered.read_only, hidden: answered.hidden };
  return { required: field.required ?? false, readOnly: false, hidden: false };
}

/** Whether a field depends on the entry: one of its conditions is set. */
export function isConditional(field: EntryFormField): boolean {
  return Boolean(field.required_when || field.read_only_when || field.hidden_when);
}

/**
 * The fields capture unfolds under its line: the ones required as the draft stands, and the ones
 * whose conditions may make them so - never the title, which is the line itself, and never one the
 * answer hides.
 */
export function unfoldedIn(form: EntryForm | undefined, state: EntryFormState | undefined): readonly EntryFormField[] {
  return (form?.fields ?? []).filter((field) => {
    if (field.key === 'title') return false;
    const standing = standingOf(field, state);
    if (standing.hidden) return false;
    return standing.required || Boolean(field.required_when);
  });
}

/** One custom field's row on the entry panel, as the form lays it out. */
export interface PanelRow {
  readonly key: string;
  /** The section's name where the row opens one. */
  readonly opens?: string;
  readonly required: boolean;
  readonly readOnly: boolean;
}

/**
 * The entry panel's custom field rows in the form's sections and order, the ones the answer hides
 * left out, and the fields the form does not name after them as they were - the same form capture
 * draws, in edit mode.
 */
export function panelRows(
  form: EntryForm | undefined,
  answer: EntryFormState | undefined,
  keys: readonly string[],
): readonly PanelRow[] {
  const rows: PanelRow[] = [];
  const placed = new Set<string>();
  for (const group of groupsOf(form)) {
    let opens = group.section?.name;
    for (const field of group.fields) {
      const custom = customKeyOf(field.key);
      if (custom === undefined || !keys.includes(custom)) continue;
      placed.add(custom);
      const standing = standingOf(field, answer);
      if (standing.hidden) continue;
      rows.push({ key: custom, ...(opens ? { opens } : {}), required: standing.required, readOnly: standing.readOnly });
      opens = undefined;
    }
  }
  for (const key of keys) if (!placed.has(key)) rows.push({ key, required: false, readOnly: false });
  return rows;
}

/** A field problem's path as a form key: `/custom_fields/size` is `custom_fields.size`. */
export function formKeyOfPath(path: string): string {
  return path.replace(/^\//, '').split('/').join('.');
}

/** A refusal's field messages by form key, so each lands on its own field. */
export function refusalsByField(fields: ReadonlyMap<string, string>): ReadonlyMap<string, string> {
  const out = new Map<string, string>();
  for (const [path, message] of fields) out.set(formKeyOfPath(path), message);
  return out;
}

/** The workspace's own words for a validation that does not hold, where it wrote any. */
export function validationWords(form: EntryForm | undefined, key: string): string | undefined {
  const words = form?.validations?.find((validation) => validation.key === key)?.message;
  return words && words.trim() !== '' ? words : undefined;
}

/** The required fields of the draft that hold nothing, so capture asks before it sends. */
export function unfilled(
  form: EntryForm | undefined,
  state: EntryFormState | undefined,
  values: { readonly notes?: string; readonly custom: Readonly<Record<string, unknown>> },
): readonly string[] {
  return unfoldedIn(form, state)
    .filter((field) => standingOf(field, state).required)
    .filter((field) => {
      const custom = customKeyOf(field.key);
      const value = custom !== undefined ? values.custom[custom] : field.key === 'notes' ? values.notes : undefined;
      if (field.key === 'due_date' || field.key === 'start_at') return false;
      return value === undefined || value === null || value === '' || (Array.isArray(value) && value.length === 0);
    })
    .map((field) => field.key);
}

// --- Editing a form --------------------------------------------------------------------------

/** A field as an editor changes it: every word a string, the default as typed. */
export interface FieldDraft {
  key: string;
  section: string;
  required: boolean;
  required_when: string;
  read_only_when: string;
  hidden_when: string;
  default: string;
  help: string;
}

/** A channel's narrowing as an editor changes it; a preset as typed. */
export interface ChannelDraft {
  channel: EntryFormChannel['channel'];
  closed: boolean;
  hidden: string[];
  presets: Record<string, string>;
}

/** A form with every list present and every word a string, for an editor that changes it in place. */
export interface FormDraft {
  sections: { key: string; name: string }[];
  fields: FieldDraft[];
  validations: { key: string; field: string; holds: string; message: string }[];
  channels: ChannelDraft[];
}

/** A stored value as an editor shows it: a list joined by commas. */
export function typedOf(value: unknown): string {
  if (value === undefined || value === null) return '';
  if (Array.isArray(value)) return value.map(String).join(', ');
  return String(value);
}

export function draftOfForm(form: EntryForm | undefined): FormDraft {
  return {
    sections: (form?.sections ?? []).map((section) => ({ key: section.key, name: section.name })),
    fields: (form?.fields ?? []).map((field) => ({
      key: field.key, section: field.section ?? '', required: field.required ?? false,
      required_when: field.required_when ?? '', read_only_when: field.read_only_when ?? '',
      hidden_when: field.hidden_when ?? '', default: typedOf(field.default), help: field.help ?? '',
    })),
    validations: (form?.validations ?? []).map((validation) => ({
      key: validation.key, field: validation.field ?? '', holds: validation.holds, message: validation.message ?? '',
    })),
    channels: (form?.channels ?? []).map((channel) => ({
      channel: channel.channel,
      closed: channel.closed ?? false,
      hidden: [...(channel.hidden ?? [])],
      presets: Object.fromEntries(Object.entries(channel.presets ?? {}).map(([key, value]) => [key, typedOf(value)])),
    })),
  };
}

/** A new field of the form, required by nothing. */
export function newField(key: string): FieldDraft {
  return { key, section: '', required: false, required_when: '', read_only_when: '', hidden_when: '', default: '', help: '' };
}

/** The keys an editor offers to add: the entry's own and the custom fields, less those in the form. */
export function addableKeys(draft: FormDraft, customKeys: readonly string[]): readonly string[] {
  const taken = new Set(draft.fields.map((field) => field.key));
  return [...OWN_FORM_KEYS, ...customKeys.map(formKeyOfCustom)].filter((key) => !taken.has(key));
}

/** Moves a field one place up or down; a move past either end changes nothing. */
export function moveField(draft: FormDraft, key: string, by: -1 | 1): void {
  const at = draft.fields.findIndex((field) => field.key === key);
  const to = at + by;
  if (at < 0 || to < 0 || to >= draft.fields.length) return;
  const [moved] = draft.fields.splice(at, 1);
  if (moved) draft.fields.splice(to, 0, moved);
}

/** Takes a field out of the form, and out of every channel and validation that names it. */
export function removeField(draft: FormDraft, key: string): void {
  draft.fields = draft.fields.filter((field) => field.key !== key);
  for (const channel of draft.channels) {
    channel.hidden = channel.hidden.filter((each) => each !== key);
    delete channel.presets[key];
  }
  for (const validation of draft.validations) if (validation.field === key) validation.field = '';
}

/** Takes a section out; its fields stay, under no section. */
export function removeSection(draft: FormDraft, key: string): void {
  draft.sections = draft.sections.filter((section) => section.key !== key);
  for (const field of draft.fields) if (field.section === key) field.section = '';
}

/** A channel's narrowing, added where the draft holds none yet. */
export function channelIn(draft: FormDraft, channel: EntryFormChannel['channel']): ChannelDraft {
  let rule = draft.channels.find((each) => each.channel === channel);
  if (!rule) {
    rule = { channel, closed: false, hidden: [], presets: {} };
    draft.channels.push(rule);
  }
  return rule;
}

/** Turns what was typed into the value a field stores - a custom field's kind decides. */
export type Shape = (key: string, typed: string) => unknown;

const asTyped: Shape = (_key, typed) => typed.trim();
const blank = (text: string) => text.trim() === '';

/**
 * The form as it is sent: empty words left out, a default and a preset only where one was typed,
 * a channel that narrows nothing dropped - so that what is stored is what somebody designed.
 */
export function formOfDraft(draft: FormDraft, shape: Shape = asTyped): EntryForm {
  return {
    sections: draft.sections.map((section) => ({ key: section.key.trim(), name: section.name.trim() })),
    fields: draft.fields.map((field) => ({
      key: field.key,
      ...(blank(field.section) ? {} : { section: field.section }),
      required: field.required,
      ...(blank(field.required_when) ? {} : { required_when: field.required_when.trim() }),
      ...(blank(field.read_only_when) ? {} : { read_only_when: field.read_only_when.trim() }),
      ...(blank(field.hidden_when) ? {} : { hidden_when: field.hidden_when.trim() }),
      ...(blank(field.default) ? {} : { default: shape(field.key, field.default) }),
      ...(blank(field.help) ? {} : { help: field.help.trim() }),
    })),
    validations: draft.validations.map((validation) => ({
      key: validation.key.trim(),
      ...(blank(validation.field) ? {} : { field: validation.field }),
      holds: validation.holds.trim(),
      ...(blank(validation.message) ? {} : { message: validation.message.trim() }),
    })),
    channels: draft.channels
      .map((channel) => {
        const presets = Object.entries(channel.presets).filter(([, typed]) => !blank(typed));
        return {
          channel: channel.channel,
          closed: channel.closed,
          ...(channel.hidden.length > 0 ? { hidden: [...channel.hidden] } : {}),
          ...(presets.length > 0 ? { presets: Object.fromEntries(presets.map(([key, typed]) => [key, shape(key, typed)])) } : {}),
        };
      })
      .filter((channel) => channel.closed || Object.keys(channel).length > 2),
  };
}

/** The built-in roles from the strongest down: the one `actor.role` names is the strongest held. */
const BUILT_IN = ['OWNER', 'ADMIN', 'MEMBER', 'CONTRIBUTOR', 'VIEWER', 'GUEST', 'AUDITOR'] as const;

/** Who writes, as far as the device knows it: what the server last explained of the reader there. */
export function actorOf(explanation: AccessExplanation | null | undefined): FormActor {
  const roles = explanation?.roles ?? [];
  const role = BUILT_IN.find((each) => roles.includes(each));
  return { ...(role ? { role } : {}), roles, permissions: explanation?.permissions ?? [], kind: 'HUMAN' };
}

/** A grant of a custom role as the reader's replica holds it (offline-sync.md §4.3). */
export interface HeldGrant {
  readonly role: string;
  readonly scope_type: string;
  readonly scope_id?: string;
}

/** A custom role's definition as the replica holds it. */
export interface HeldRole {
  readonly key: string;
  readonly permissions?: readonly string[];
}

/**
 * The actor with the custom roles the replica says the reader holds along the path - the
 * workspace, the hub, the collection - and what those roles carry. What the server last explained
 * stays; the replica adds what was granted since, and is what a device without an answer has.
 */
export function withOwnGrants(
  actor: FormActor, grants: readonly HeldGrant[], defined: readonly HeldRole[], path: readonly string[],
): FormActor {
  const along = grants.filter((grant) => grant.scope_type === 'TENANT' || (grant.scope_id !== undefined && path.includes(grant.scope_id)));
  if (along.length === 0) return actor;
  const roles = new Set(actor.roles ?? []);
  const permissions = new Set(actor.permissions ?? []);
  for (const grant of along) {
    roles.add(grant.role);
    for (const permission of defined.find((role) => role.key === grant.role)?.permissions ?? []) permissions.add(permission);
  }
  return { ...actor, roles: [...roles], permissions: [...permissions] };
}

/**
 * How a type's form stands for a draft while the server cannot answer: the entry the copy holds
 * with the draft's values over it, judged as the push will be judged - through `SYNC`. `undefined`
 * where the type has no form or a condition cannot be answered here; the editor then draws the form
 * as it stands.
 */
export function offlineFormState(
  type: EntryType | undefined, draft: EntryFormDraft, held: WorkItem | undefined, actor: FormActor,
): EntryFormState | undefined {
  if (!type || !hasForm(type.form)) return undefined;
  const custom = { ...((held?.custom_fields as Record<string, unknown> | undefined) ?? {}), ...(draft.custom_fields ?? {}) };
  return evaluateForm(type.form, type.key, 'SYNC', {
    title: draft.title ?? held?.title ?? '',
    notes: draft.notes ?? held?.notes ?? '',
    level: type.level,
    completed: held?.completion?.is_completed ?? false,
    dueAt: held?.due_at ?? null,
    startAt: held?.start_at ?? null,
    customFields: custom,
  }, actor);
}
