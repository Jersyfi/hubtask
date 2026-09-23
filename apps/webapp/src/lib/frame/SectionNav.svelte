<!-- SPDX-License-Identifier: BUSL-1.1
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

  import { SideNav } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import type { SectionGroup } from '../navigation.ts';

  interface Props {
    /** What this navigation is called, for the landmark. Resolved text (ADR-0011). */
    label: string;
    /** The section's list, in its groups. */
    groups: readonly SectionGroup[];
    /** The row the reader is on, by the id the list gives it. The frame decides, because it has the route. */
    current?: string;
    /** Folded to its marks, from `expanded` up: the same list, drawing its marks alone. */
    isRail?: boolean;
    onnavigate: (path: string) => void;
  }

  const { label, groups, current, isRail = false, onnavigate }: Props = $props();

  // The groups as one list: each group's first row opens a band, and the way back opens none - it
  // is one row above everything and needs no caption to say so.
  const nodes = $derived(
    groups.flatMap((group) =>
      group.rows.map((row, index) => ({
        id: row.id,
        label: t(row.code),
        icon: row.icon,
        ...(index === 0 && group.code ? { band: { caption: t(group.code) } } : {}),
      })),
    ),
  );

  function navigate(id: string) {
    const row = groups.flatMap((group) => group.rows).find((each) => each.id === id);
    if (row) onnavigate(row.path);
  }
</script>

<SideNav {label} {nodes} {current} {isRail} onnavigate={navigate} />
