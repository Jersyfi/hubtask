// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The arithmetic of the inventory's three scenes beside the stock (work-surface §6.10): one
 * service's map by depth with only its problems if asked (`invmap`), the upkeep's six figures and
 * its to-dos with one act each and the sources with their runs (`invcare`, IN-4), and the
 * register's summary and warranty tone (`invreg`). Pure, so the screens draw and the tests hold the
 * same answers.
 */

import type { CiAsset, CiConnector, CiDuplicate, CiHold, CiRun, CiWorkEntry, CmdbHealth, ServiceMap } from '@hubtask/sync-engine';

// The map of one service (scene invmap).

/** How far below the service the map reaches: *1 step · 2 · 3*. */
export const MAP_DEPTHS = [1, 2, 3] as const;
export type MapDepth = (typeof MAP_DEPTHS)[number];

type MapNode = ServiceMap['nodes'][number];

export interface GraphNode extends MapNode {
  /** Steps below the service; the service is 0. */
  readonly depth: number;
}

export interface ServiceGraph {
  /** The columns from the service to the deepest drawn, each node ordered so lines cross less. */
  readonly columns: readonly (readonly GraphNode[])[];
  /** What rests on what, forward only: an edge's end lies deeper than its start. */
  readonly edges: readonly { readonly key: string; readonly source: string; readonly target: string; readonly isGroup: boolean }[];
}

/**
 * One service and what it rests on, `depth` steps down at most, each object in the column of its
 * shortest way from the service. With `troubled`, only the objects on a way to one of them stay - the
 * service always. An edge back up or within a column is not drawn: the list beside the picture
 * names every relation.
 */
export function serviceGraph(
  map: Pick<ServiceMap, 'nodes' | 'edges'>,
  serviceId: string,
  depth: number,
  troubled?: ReadonlySet<string>,
): ServiceGraph {
  const nodes = new Map(map.nodes.map((node) => [node.ci_id, node]));
  if (!nodes.has(serviceId)) return { columns: [], edges: [] };
  const below = new Map<string, string[]>();
  for (const edge of map.edges) below.set(edge.source, [...(below.get(edge.source) ?? []), edge.target]);
  const reached = new Map<string, number>([[serviceId, 0]]);
  let front = [serviceId];
  for (let step = 1; step <= depth && front.length > 0; step += 1) {
    const next: string[] = [];
    for (const id of front) {
      for (const to of below.get(id) ?? []) {
        if (reached.has(to) || !nodes.has(to)) continue;
        reached.set(to, step);
        next.push(to);
      }
    }
    front = next;
  }
  let edges = map.edges.filter((edge) => reached.has(edge.source) && reached.has(edge.target) && reached.get(edge.target)! > reached.get(edge.source)!);
  if (troubled) {
    const leads = towards(edges, new Set([...troubled].filter((id) => reached.has(id))));
    for (const id of [...reached.keys()]) if (id !== serviceId && !leads.has(id)) reached.delete(id);
    edges = edges.filter((edge) => reached.has(edge.source) && reached.has(edge.target));
  }
  const columns: GraphNode[][] = [];
  const place = new Map<string, number>();
  const deepest = Math.max(...reached.values());
  for (let at = 0; at <= deepest; at += 1) {
    const here = [...reached].filter(([, d]) => d === at).map(([id]) => ({ ...nodes.get(id)!, depth: at }));
    const centre = (node: GraphNode) => {
      const above = edges.filter((edge) => edge.target === node.ci_id).map((edge) => place.get(edge.source)).filter((p): p is number => p !== undefined);
      return above.length > 0 ? above.reduce((sum, p) => sum + p, 0) / above.length : Number.POSITIVE_INFINITY;
    };
    const ordered = here
      .map((node) => ({ node, centre: centre(node) }))
      .sort((a, b) => (a.centre === b.centre ? 0 : a.centre - b.centre) || a.node.name.localeCompare(b.node.name))
      .map((each) => each.node);
    ordered.forEach((node, index) => place.set(node.ci_id, index));
    if (ordered.length > 0) columns.push(ordered);
  }
  return {
    columns,
    edges: edges.map((edge) => ({ key: `${edge.source}>${edge.target}:${edge.type}`, source: edge.source, target: edge.target, isGroup: edge.type.startsWith('group:') })),
  };
}

/** The objects from which one of `goals` is reached along `edges`, the goals included. */
function towards(edges: readonly { readonly source: string; readonly target: string }[], goals: ReadonlySet<string>): Set<string> {
  const leads = new Set(goals);
  let isGrowing = true;
  while (isGrowing) {
    isGrowing = false;
    for (const edge of edges) {
      if (leads.has(edge.target) && !leads.has(edge.source)) {
        leads.add(edge.source);
        isGrowing = true;
      }
    }
  }
  return leads;
}

/** The edges on a way from the service down to an object with an open incident: drawn dashed. */
export function troublePath(graph: ServiceGraph, struck: ReadonlySet<string>): Set<string> {
  const leads = towards(graph.edges, struck);
  return new Set(graph.edges.filter((edge) => leads.has(edge.target)).map((edge) => edge.key));
}

/** A change, or anything with a window set: *Geplante Changes* lists it, not *Offen*. */
export const isPlanned = (entry: Pick<CiWorkEntry, 'entry_type' | 'start_at'>) =>
  entry.entry_type.toLowerCase().includes('change') || Boolean(entry.start_at);

/** The open work on a service (IN-3): the incidents and tickets, and what is planned. */
export function serviceWork(entries: readonly CiWorkEntry[]): { readonly open: readonly CiWorkEntry[]; readonly planned: readonly CiWorkEntry[] } {
  const live = entries.filter((each) => !each.completed);
  return {
    open: live.filter((each) => !isPlanned(each)),
    planned: live.filter(isPlanned).sort((a, b) => (a.start_at ?? a.due_at ?? '').localeCompare(b.start_at ?? b.due_at ?? '')),
  };
}

/**
 * What the map marks on each object, by its id: the first open incident or ticket and the first
 * planned change. An entry naming the service stands at the service; one beneath it at the nearest
 * object it names (`via`).
 */
export function workMarks(entries: readonly CiWorkEntry[], serviceId: string): ReadonlyMap<string, { readonly struck?: CiWorkEntry; readonly planned?: CiWorkEntry }> {
  const marks = new Map<string, { struck?: CiWorkEntry; planned?: CiWorkEntry }>();
  for (const entry of entries) {
    if (entry.completed) continue;
    const at = entry.reason === 'NAMES' ? serviceId : entry.via?.ci_id;
    if (!at) continue;
    const mark = marks.get(at) ?? {};
    if (isPlanned(entry)) mark.planned ??= entry;
    else mark.struck ??= entry;
    marks.set(at, mark);
  }
  return marks;
}

// Upkeep (scene invcare, IN-4).

export type FigureId = 'complete' | 'correct' | 'stale' | 'orphans' | 'duplicates' | 'without_owner';

export interface HealthFigure {
  readonly id: FigureId;
  /** A share in whole per cent for the two shares, a count for the rest. */
  readonly value: number;
  readonly isShare: boolean;
  /** Below 100 % or above none: drawn in the warning tone. */
  readonly isWanting: boolean;
  /** What the line under it counts: the incomplete objects, the deviations from a source. */
  readonly detail: number;
}

const share = (value: number | undefined) => Math.round(Math.min(1, Math.max(0, value ?? 1)) * 100);

/**
 * The six figures of the upkeep (scene invcare 1), in the mockup's order. A count a health answer
 * lacks counts none; a share it lacks is whole.
 */
export function healthFigures(health: Partial<CmdbHealth>, holds: number): HealthFigure[] {
  const count = (value: number | undefined) => (typeof value === 'number' ? value : 0);
  const incomplete = (health.problems ?? []).filter((each) => each.kinds.includes('INCOMPLETE')).length;
  const complete = share(health.completeness);
  const correct = share(health.correctness);
  const counted = (id: FigureId, value: number): HealthFigure => ({ id, value, isShare: false, isWanting: value > 0, detail: value });
  return [
    { id: 'complete', value: complete, isShare: true, isWanting: complete < 100, detail: incomplete },
    { id: 'correct', value: correct, isShare: true, isWanting: correct < 100 || holds > 0, detail: holds },
    counted('stale', count(health.stale)),
    counted('orphans', count(health.orphans)),
    counted('duplicates', count(health.duplicates)),
    counted('without_owner', count(health.without_owner)),
  ];
}

export type CareKind = 'NO_OWNER' | 'ORPHAN' | 'REVIEW_OVERDUE' | 'HOLD' | 'DUPLICATE' | 'STALE' | 'INCOMPLETE';
/** The one act of a to-do: *Festlegen*, *Beziehung*, *Prüfen*, *Vergleichen*, *Entscheiden*. */
export type CareAct = 'owner' | 'relate' | 'review' | 'compare' | 'decide';

export interface CareTodo {
  readonly key: string;
  readonly kind: CareKind;
  readonly act: CareAct;
  /** The object it is about; a duplicate's first. */
  readonly ciId?: string;
  readonly hold?: CiHold;
  readonly duplicate?: CiDuplicate;
  /** The required attributes an incomplete object lacks. */
  readonly missing?: readonly string[];
}

const KIND_ORDER: readonly CareKind[] = ['NO_OWNER', 'ORPHAN', 'REVIEW_OVERDUE', 'HOLD', 'DUPLICATE', 'STALE', 'INCOMPLETE'];
const ACT: Readonly<Record<CareKind, CareAct>> = {
  NO_OWNER: 'owner', ORPHAN: 'relate', REVIEW_OVERDUE: 'review', HOLD: 'compare', DUPLICATE: 'decide', STALE: 'review', INCOMPLETE: 'review',
};

/**
 * The upkeep's to-dos (scene invcare 2): one row per problem the health finds on an object, per
 * item a source's run held, and per open duplicate pair - the pair, not the health's mark on each of
 * its objects, which would ask the same question twice. Ordered by what a person settles fastest
 * and what the rest depends on: responsibility, relations, reviews, then the sources' questions.
 */
export function careTodos(
  health: Pick<CmdbHealth, 'problems'> | undefined,
  holds: readonly CiHold[],
  duplicates: readonly CiDuplicate[],
): CareTodo[] {
  const out: CareTodo[] = [];
  for (const problem of health?.problems ?? []) {
    for (const kind of problem.kinds) {
      if (kind === 'DUPLICATE') continue;
      out.push({ key: `${kind}:${problem.ci_id}`, kind, act: ACT[kind], ciId: problem.ci_id, ...(kind === 'INCOMPLETE' ? { missing: problem.missing ?? [] } : {}) });
    }
  }
  for (const hold of holds) out.push({ key: `HOLD:${hold.id}`, kind: 'HOLD', act: 'compare', hold });
  for (const duplicate of duplicates) out.push({ key: `DUPLICATE:${duplicate.id}`, kind: 'DUPLICATE', act: 'decide', ciId: duplicate.ci_ids[0], duplicate });
  return out
    .map((todo, at) => ({ todo, at }))
    .sort((a, b) => KIND_ORDER.indexOf(a.todo.kind) - KIND_ORDER.indexOf(b.todo.kind) || a.at - b.at)
    .map((each) => each.todo);
}

/** How many to-dos stand before *Show all n*. */
export const TODOS_SHOWN = 5;

/** A connector that brings objects, not alerts: a pull, or an intake posting objects. */
export const bringsObjects = (connector: Pick<CiConnector, 'push' | 'inventory'>) => !connector.push || connector.inventory;

export interface SourceRow {
  readonly key: string;
  /** The connector's key, or the source a run named without a connector - a file's import. */
  readonly name: string;
  readonly connector?: CiConnector;
  readonly lastAt?: string;
  readonly run?: CiRun;
  readonly read?: number;
  readonly changed?: number;
  readonly nextAt?: string;
}

/**
 * *Quellen und Läufe* (scene invcare 4): each connector bringing objects with its latest run - read
 * and changed - and its next, then each source a run named that no connector stands for, newest
 * first. The latest run of a source is the first in the runs' order, which is newest first.
 */
export function sourceRows(connectors: readonly CiConnector[], runs: readonly CiRun[]): SourceRow[] {
  const latest = new Map<string, CiRun>();
  for (const run of runs) if (!run.dry_run && !latest.has(run.source)) latest.set(run.source, run);
  const counts = (run: CiRun | undefined) => (run ? { read: readOf(run), changed: run.counts.created + run.counts.updated } : {});
  const rows: SourceRow[] = connectors.filter(bringsObjects).map((connector) => {
    const run = latest.get(connector.source);
    return {
      key: `connector:${connector.id}`,
      name: connector.key,
      connector,
      ...(connector.last_run_at ?? run?.started_at ? { lastAt: (connector.last_run_at ?? run?.started_at)! } : {}),
      ...(run ? { run } : {}),
      ...counts(run),
      ...(connector.next_run_at && connector.enabled ? { nextAt: connector.next_run_at } : {}),
    };
  });
  const named = new Set(connectors.map((each) => each.source));
  for (const [source, run] of latest) {
    if (named.has(source)) continue;
    rows.push({ key: `source:${source}`, name: source, lastAt: run.started_at, run, ...counts(run) });
  }
  return rows;
}

const readOf = (run: CiRun) =>
  typeof run.counts.items === 'number' && run.counts.items > 0
    ? run.counts.items
    : run.counts.created + run.counts.updated + run.counts.unchanged + run.counts.held + run.counts.duplicates + run.counts.refused;

/** An object no source but a person has ever spoken for: *Von Hand*. */
export const isByHand = (ci: { readonly provenance?: Readonly<Record<string, { readonly source: string }>> }) =>
  Object.values(ci.provenance ?? {}).every((each) => each.source === 'manual');

// The register (scene invreg).

/** Months between two plain days, counted whole: from 14 March to 13 July is three. */
export function monthsUntil(today: string, until: string): number {
  const [y1, m1, d1] = today.split('-').map(Number) as [number, number, number];
  const [y2, m2, d2] = until.split('-').map(Number) as [number, number, number];
  return (y2 - y1) * 12 + (m2 - m1) - (d2 < d1 ? 1 : 0);
}

/** How near a warranty's end must be to be drawn as ending. */
export const WARRANTY_SOON_MONTHS = 6;

/** A device in history - retired or disposed - is no longer watched for its warranty. */
const isHistory = (asset: Pick<CiAsset, 'state'>) => asset.state === 'RETIRED' || asset.state === 'DISPOSED';

/** The warranty's tone: ended, or ending within six months, on a device still kept. */
export function warrantyTone(asset: Pick<CiAsset, 'state' | 'warranty_until' | 'warranty_ended'>, today: string): 'warning' | 'neutral' {
  if (isHistory(asset)) return 'neutral';
  if (asset.warranty_ended) return 'warning';
  if (!asset.warranty_until) return 'neutral';
  return asset.warranty_until < today || monthsUntil(today, asset.warranty_until) < WARRANTY_SOON_MONTHS ? 'warning' : 'neutral';
}

/**
 * The register's summary line (scene invreg 1): the cost of what is issued, per currency in the
 * order met, and the warranties ending within six months - how many and in how many months the first.
 */
export function registerSummary(
  assets: readonly Pick<CiAsset, 'state' | 'cost_minor' | 'currency' | 'warranty_until' | 'warranty_ended'>[],
  today: string,
): { readonly inUse: readonly { readonly currency: string; readonly minor: number }[]; readonly ending: number; readonly firstInMonths?: number } {
  const inUse = new Map<string, number>();
  for (const asset of assets) {
    if (asset.state !== 'DEPLOYED' || !asset.cost_minor) continue;
    inUse.set(asset.currency, (inUse.get(asset.currency) ?? 0) + asset.cost_minor);
  }
  const ending = assets
    .filter((asset) => !isHistory(asset) && !asset.warranty_ended && asset.warranty_until && asset.warranty_until >= today)
    .map((asset) => monthsUntil(today, asset.warranty_until!))
    .filter((months) => months < WARRANTY_SOON_MONTHS)
    .sort((a, b) => a - b);
  return {
    inUse: [...inUse].map(([currency, minor]) => ({ currency, minor })),
    ending: ending.length,
    ...(ending.length > 0 ? { firstInMonths: ending[0] } : {}),
  };
}

/** The moves a register row offers as buttons, in the order a person walks them (scene invreg 2). */
export const REGISTER_ACTS: readonly CiAsset['state'][] = ['ORDERED', 'IN_STOCK', 'DEPLOYED', 'RETIRED', 'DISPOSED'];
