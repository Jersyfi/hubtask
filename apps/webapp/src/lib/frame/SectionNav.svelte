<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A section's own navigation column (ADR-0063 decision 7, ADR-0065 decision 3).
  //
  // **It replaces the workspace's tree rather than joining it.** While the route's area is a
  // section the reader is in a place of its own, not in a corner of the workspace; a tree of hubs
  // beside sixteen settings screens would say they are somewhere they are not, and the tree's
  // reads - one level per open hub - would run for a reader who is not looking at any of it.
  //
  // **The way out is the first row.** A section somebody cannot leave is a trap, and the row that
  // leads back is looked for at the top rather than at the foot. It leads to the overview, which
  // is where every reader can go from.
  //
  // One `SideNav` and one list, banded the way the workspace's is: the bands are a mark on the
  // first row of each rather than five components, so the keyboard walks the section in one pass.
  //
  // It knows no section. The administration and Your settings hand it their own `SectionGroup[]`,
  // which is what makes "a section" a shape rather than a screen with a copy of a column beside
  // it; who reaches either is decided by the frame, not here.

  import { untrack } from 'svelte';

  import { SideNav } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import type { SectionGroup } from '../navigation.ts';

  interface Props {
    /** What this navigation is called, for the landmark. Resolved text (ADR-0011). */
    label: string;
    /** The section's list, in its groups. */
    groups: readonly SectionGroup[];
    /** A short figure at the end of a row, by the row's id: what needs attention there (user-experience.md §7.10). */
    figures?: Readonly<Record<string, string>>;
    /** The row the reader is on, by the id the list gives it. The frame decides, because it has the route. */
    current?: string;
    /** Folded to its marks, from `expanded` up: the same list, drawing its marks alone. */
    isRail?: boolean;
    onnavigate: (path: string) => void;
  }

  const { label, groups, figures = {}, current, isRail = false, onnavigate }: Props = $props();

  // The groups as one list: each group's first row opens a band, and the way back opens none - it
  // is one row above everything and needs no caption to say so. A group's folded rows go under one
  // *More* at its end (user-experience.md §8.2), a branch the reader opens; the reader standing on
  // one of them finds it open.
  const nodeOf = (row: SectionGroup['rows'][number]) => ({
    id: row.id,
    label: t(row.code),
    icon: row.icon,
    ...(figures[row.id] ? { meta: figures[row.id] } : {}),
  });
  const moreId = (group: SectionGroup) => `more-${group.id}`;
  const nodes = $derived(
    groups.flatMap((group) => {
      const shown = group.rows.filter((row) => !row.isFolded);
      const folded = group.rows.filter((row) => row.isFolded);
      const band = group.code ? { band: { caption: t(group.code) } } : {};
      const rows = shown.map((row, index) => ({ ...nodeOf(row), ...(index === 0 ? band : {}) }));
      if (folded.length === 0) return rows;
      const more = {
        id: moreId(group),
        label: t('app.admin.more'),
        icon: 'ellipsis' as const,
        children: folded.map(nodeOf),
        ...(shown.length === 0 ? band : {}),
      };
      return [...rows, more];
    }),
  );

  let expanded = $state<string[]>([]);
  $effect(() => {
    const holding = groups.find((group) => group.rows.some((row) => row.isFolded && row.id === current));
    if (holding && !untrack(() => expanded).includes(moreId(holding))) expanded = [...untrack(() => expanded), moreId(holding)];
  });

  function navigate(id: string) {
    const row = groups.flatMap((group) => group.rows).find((each) => each.id === id);
    if (row) onnavigate(row.path);
  }
</script>

<SideNav {label} {nodes} {current} {isRail} bind:expanded onnavigate={navigate} />
