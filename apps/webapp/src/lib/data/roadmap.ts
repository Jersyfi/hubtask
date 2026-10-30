// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A portfolio's roadmap turned into the timeline's rows (planning.md §2): a lane per hub, each
 * project under it as the span the portfolio plans it for, and the project's top-level entries
 * under that as the bars its plan schedules. Pure, so the order and the spans are checked by
 * `node --test`, as `planning.ts` is.
 *
 * **The server schedules; this module only orders.** Every bar's dates are the ones the project's
 * own plan answered; a client that moved them would be a second scheduler.
 */

import type { PortfolioRoadmap } from '@hubtask/sync-engine';

/** One row of the roadmap's chart and of the table beside it. */
export interface RoadmapRow {
  readonly id: string;
  readonly title: string;
  readonly start?: string;
  readonly due?: string;
  readonly kind?: 'milestone' | 'summary';
  readonly isCritical: boolean;
  readonly depth: number;
  /** `lane` a hub, `project` a hub or collection of the portfolio, `entry` a bar. */
  readonly level: 'lane' | 'project' | 'entry';
  readonly isDone: boolean;
  /** The project's health, for a project row. */
  readonly status?: string;
  /** Where a press on the row leads. */
  readonly href?: string;
}

const earliest = (days: readonly (string | null | undefined)[]) => days.filter((day): day is string => !!day).sort()[0];
const latest = (days: readonly (string | null | undefined)[]) => days.filter((day): day is string => !!day).sort().at(-1);

/**
 * The rows in the roadmap's order: each lane, then its projects, then each project's bars. A
 * project's span is the dates the portfolio plans it for, else its bars' reach; a lane spans its
 * projects. `unnamed` names a lane whose hub the reader may not read.
 */
export function roadmapRows(roadmap: PortfolioRoadmap, unnamed: string): RoadmapRow[] {
  const rows: RoadmapRow[] = [];
  for (const lane of roadmap.lanes) {
    const projects = lane.projects.map((project) => {
      const start = project.start_on ?? earliest(project.bars.map((bar) => bar.start));
      const due = project.end_on ?? latest([...project.bars.map((bar) => bar.finish), project.finish]);
      return { project, start, due };
    });
    rows.push({
      id: `lane:${lane.hub_id ?? rows.length}`,
      title: lane.name || unnamed,
      start: earliest(projects.map((each) => each.start)),
      due: latest(projects.map((each) => each.due)),
      kind: 'summary',
      isCritical: false,
      depth: 0,
      level: 'lane',
      isDone: false,
      href: lane.hub_id && lane.name ? `/hubs/${lane.hub_id}/plan` : undefined,
    });
    for (const { project, start, due } of projects) {
      rows.push({
        id: `project:${project.container_id}`,
        title: project.name,
        start,
        due,
        kind: 'summary',
        isCritical: false,
        depth: 1,
        level: 'project',
        isDone: false,
        status: project.status,
        href: `/${project.kind === 'HUB' ? 'hubs' : 'collections'}/${project.container_id}/plan`,
      });
      for (const bar of project.bars) {
        rows.push({
          id: bar.item_id,
          title: bar.title,
          start: bar.milestone ? undefined : bar.start,
          due: bar.finish,
          kind: bar.milestone ? 'milestone' : undefined,
          isCritical: bar.critical,
          depth: 2,
          level: 'entry',
          isDone: bar.done,
          href: `/items/${bar.item_id}`,
        });
      }
    }
  }
  return rows;
}
