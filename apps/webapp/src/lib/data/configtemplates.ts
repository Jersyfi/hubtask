// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The *Templates* gallery's arithmetic (configuration.md §6): the four origins in the order
 * the gallery draws them, what a template offers the workspace - to take it, to update it, or
 * nothing because the version in force is the newest -, and which of an upgrade's changes are
 * taken before the person chooses: every one the workspace did not change itself, never a conflict.
 */

export type Origin = 'HUBTASK' | 'INSTALLATION' | 'WORKSPACE' | 'OWN';

export const ORIGINS: readonly Origin[] = ['HUBTASK', 'INSTALLATION', 'WORKSPACE', 'OWN'];

interface Offered {
  readonly origin: Origin;
  readonly taken_version?: number | null;
  readonly update_available: boolean;
}

/** The templates by origin, each origin in the gallery's order, an empty origin left out. */
export function byOrigin<T extends { readonly origin: Origin }>(templates: readonly T[]): { origin: Origin; templates: T[] }[] {
  return ORIGINS.map((origin) => ({ origin, templates: templates.filter((each) => each.origin === origin) }))
    .filter((group) => group.templates.length > 0);
}

/** What the gallery offers for a template. */
export function offerOf(template: Offered): 'TAKE' | 'UPDATE' | 'IN_USE' {
  if (template.taken_version === null || template.taken_version === undefined) return 'TAKE';
  return template.update_available ? 'UPDATE' : 'IN_USE';
}

/** The upgrade's changes taken before the person chooses: every TAKE, no CONFLICT. */
export function takenFirst(upgrade: readonly { readonly path: string; readonly state: 'TAKE' | 'CONFLICT' }[]): string[] {
  return upgrade.filter((change) => change.state === 'TAKE').map((change) => change.path);
}

interface Asked {
  readonly key: string;
  readonly default?: string | null;
}

/**
 * The answers a plan is asked with: each question's own answer, its default where the person left
 * it, and nothing for one left open with no default - the server then leaves out what needs it.
 */
export function answersOf(questions: readonly Asked[] | undefined, given: Readonly<Record<string, string>>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const question of questions ?? []) {
    const value = (given[question.key] ?? '').trim() || (question.default ?? '');
    if (value) out[question.key] = value;
  }
  return out;
}

interface WithSamples extends Offered {
  readonly hub_document?: { readonly [section: string]: unknown } | null;
}

/**
 * Whether taking the template offers its sample entries: only to a hub, only where its hub part
 * brings some, and only on the hub's first take - an upgrade neither adds nor removes them.
 */
export function offersSamples(template: WithSamples | undefined, hubId: string | undefined): boolean {
  if (!template || !hubId || offerOf(template) !== 'TAKE') return false;
  const samples = template.hub_document?.['samples'];
  return typeof samples === 'object' && samples !== null && Object.keys(samples).length > 0;
}

/**
 * Which gallery a settings page draws: a collection's - the templates with a collection part -
 * before its hub's, a hub's - those with a hub part - before the workspace's.
 */
export function galleryScope(hubId: string | undefined, collectionId: string | undefined): 'collection' | 'hub' | 'workspace' {
  if (collectionId) return 'collection';
  return hubId ? 'hub' : 'workspace';
}

/** A plan's refusal or change as the contract answers it, what the words below read of it. */
interface Refused { readonly path: string; readonly code: string; readonly params?: Readonly<Record<string, string>> }
interface Changed {
  readonly section: string;
  readonly key: string;
  readonly op: 'ADD' | 'CHANGE' | 'REMOVE';
  readonly before?: Readonly<Record<string, unknown>> | null;
  readonly after?: Readonly<Record<string, unknown>> | null;
}

/** The configuration's sections a template may bring, each with a word for it (`app.templates.section_*`). */
export const SECTIONS: readonly string[] = [
  'modules', 'roles', 'fields', 'link_types', 'types', 'workflows', 'settings', 'structure', 'scheme', 'tolerance',
  'business_calendars', 'sla_policies', 'reply_templates', 'requester_organisations', 'views', 'rules', 'pages',
  'template_pages', 'collection_structure', 'collection_tolerance', 'collection_views', 'collection_rules', 'collection_fields',
  'collection_attributes',
];

/** The word for a section, a generic one for a section this client does not know yet. */
export const sectionWord = (section: string): string =>
  SECTIONS.includes(section) ? `app.templates.section_${section}` : 'app.templates.section_other';

/**
 * The modules a plan is refused for because they are off, in the order first named - and only
 * where nothing else refuses it, since switching them on would then not make it usable.
 */
export function modulesToSwitchOn(refusals: readonly Refused[]): string[] {
  if (refusals.length === 0 || !refusals.every((each) => each.code === 'module.off' && each.params?.module)) return [];
  return [...new Set(refusals.map((each) => each.params?.module ?? ''))];
}

/**
 * Whether a place's plan was refused because the workspace has to take the template first: the
 * server says so as a field of `config.refused`, not as the problem's own code.
 */
export function asksWorkspaceFirst(error: {
  readonly detailCode?: string;
  readonly fieldErrors?: readonly { readonly path?: string; readonly code?: string }[];
}): boolean {
  const code = 'config.template_workspace_first';
  return error.detailCode === code || (error.fieldErrors ?? []).some((each) => each.code === code);
}

/**
 * Whether a place's template opens on the workspace part's plan (user-experience.md §5.2): the
 * list says the workspace has not taken it, and the reader may change the workspace - so the
 * refusal never comes after choosing.
 */
export function startsWithWorkspace(template: { readonly workspace_first?: boolean | null }, place: string | undefined, mayChangeWorkspace: boolean): boolean {
  return place !== undefined && mayChangeWorkspace && template.workspace_first === true;
}

/** A change of a plan in words: what it does to which part, as code and parameters. */
export function changeWords(change: Changed): { code: string; section: string; key: string; module?: string } {
  const code = `app.templates.change_${change.op.toLowerCase()}`;
  // A module is named by its product's word, a block with a name by that name (§5.4).
  if (change.section === 'modules') {
    // Said as the switch it is: *Switches on Pages*, not a change of a module.
    const to = change.after?.['switch'];
    const said = to === 'ON' ? 'app.templates.change_module_on' : to === 'OFF' ? 'app.templates.change_module_off' : code;
    return { code: said, section: sectionWord(change.section), key: change.key, module: `app.modules.name_${change.key.toLowerCase()}` };
  }
  const named = [change.after?.['name'], change.before?.['name']].find((each): each is string => typeof each === 'string' && each.trim() !== '');
  return { code, section: sectionWord(change.section), key: named ?? change.key };
}

/** A refusal in words: the section it is about, and the server's message code for why. */
export function refusalWords(refused: Refused): { section: string; code: string; module?: string } {
  const section = sectionWord(refused.path.split('/').filter(Boolean)[0] ?? '');
  if (refused.code === 'module.off' && refused.params?.module) {
    return { section, code: 'app.templates.needs_module', module: `app.modules.name_${refused.params.module.toLowerCase()}` };
  }
  return { section, code: refused.code };
}

type Translate = (code: string, params?: Readonly<Record<string, string | number>>) => string;

/**
 * A plan as `PlanSummary`'s lines (user-experience.md §5.4): each refusal first, then each change,
 * a sentence with the block's name - never the document's path or its JSON.
 */
export function planLines(
  plan: { readonly changes: readonly Changed[]; readonly refusals: readonly Refused[] },
  t: Translate,
): { text: string; tone: 'add' | 'change' | 'remove' | 'refuse' }[] {
  const refused = plan.refusals.map((each) => {
    const words = refusalWords(each);
    const reason = words.module ? t(words.code, { module: t(words.module) }) : t(words.code, each.params ?? {});
    return { text: t('app.templates.refused_item', { section: t(words.section), reason }), tone: 'refuse' as const };
  });
  const changed = plan.changes.map((each) => {
    const words = changeWords(each);
    const tone = each.op === 'ADD' ? ('add' as const) : each.op === 'REMOVE' ? ('remove' as const) : ('change' as const);
    return { text: t(words.code, { section: t(words.section), key: words.module ? t(words.module) : words.key }), tone };
  });
  return [...refused, ...changed];
}
