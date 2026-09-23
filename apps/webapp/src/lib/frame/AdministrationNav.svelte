<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The administration's own navigation column (ADR-0063 decision 7).
  //
  // **It replaces the workspace's tree rather than joining it.** While the route's area is
  // `administration` the reader is in a section, not in a corner of the workspace; a tree of hubs
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
  // Who reaches this is not decided here. The area is offered where `GET /quotas` is not refused,
  // which is the frame's question and the area's condition exactly (ADR-0061 decision 1).

  import { SideNav } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import { ADMINISTRATION } from '../navigation.ts';

  interface Props {
    /** The row the reader is on, by the id the list gives it. The frame decides, because it has the route. */
    current?: string;
    /** Folded to its marks, from `expanded` up: the same list, drawing its marks alone. */
    isRail?: boolean;
    onnavigate: (path: string) => void;
  }

  const { current, isRail = false, onnavigate }: Props = $props();

  // The five groups as one list: each group's first row opens a band, and the way back opens
  // none - it is one row above everything and needs no caption to say so.
  const nodes = $derived(
    ADMINISTRATION.flatMap((group) =>
      group.rows.map((row, index) => ({
        id: row.id,
        label: t(row.code),
        icon: row.icon,
        ...(index === 0 && group.code ? { band: { caption: t(group.code) } } : {}),
      })),
    ),
  );

  function navigate(id: string) {
    const row = ADMINISTRATION.flatMap((group) => group.rows).find((each) => each.id === id);
    if (row) onnavigate(row.path);
  }
</script>

<SideNav label={t('app.admin.nav')} {nodes} {current} {isRail} onnavigate={navigate} />
