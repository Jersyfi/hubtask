<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one head of a hub tool's tabs (the navigation concept §2.4, §3): the trail to the hub, the
  // tool's name - a queue's on *Queues*, with its ☆ -, one figure slot, the tab's actions, then the
  // tool's bar at the offset a collection's stands at (`PlaceHead`), so switching *Schedule*,
  // *Releases* and *Sprints* moves neither the title nor the bar.
  import type { Snippet } from 'svelte';
  import type { MenuItem, PageAction } from '@hubtask/design-system/components';

  import { containers } from '../data/containers.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { HUB_TOOLS, toolPath, type HubTool } from '../navigation.ts';
  import PlaceHead from './PlaceHead.svelte';
  import PlaceViews from './PlaceViews.svelte';

  interface Props {
    hubId: string;
    tool: HubTool['id'];
    /** The tab the screen is, as the tool's bar names it. */
    current?: string;
    onnavigate: (path: string) => void;
    /** Where the tool's title is something it holds - a queue's name. */
    title?: string;
    titleEnd?: Snippet;
    figure?: Snippet;
    primary?: PageAction;
    secondary?: readonly [] | readonly [PageAction] | readonly [PageAction, PageAction];
    menu?: { label: string; items: readonly MenuItem[]; onselect: (id: string) => void; opener?: string };
    /** The tool's own bar where its tabs are its own (*Inventory*'s sections); else its tabs. */
    bar?: Snippet;
    notices?: Snippet;
  }

  const { hubId, tool, current = '', onnavigate, title, titleEnd, figure, primary, secondary, menu, bar, notices }: Props = $props();

  const hub = $derived(containers.find(hubId));
  const name = $derived(t(HUB_TOOLS.find((each) => each.id === tool)?.code ?? ''));
  const trail = $derived([
    ...(hub ? [{ id: hub.id, label: hub.name, href: `/hubs/${hub.id}` }] : []),
    { id: tool, label: name },
  ]);
</script>

<PlaceHead
  kind="tool"
  title={title ?? name}
  {trail}
  trailLabel={t('app.product_home.trail')}
  expandLabel={t('app.admin.expand_trail')}
  oncrumb={(crumbId) => onnavigate(crumbId === hubId ? `/hubs/${hubId}` : toolPath(hubId, tool))}
  {titleEnd}
  {figure}
  {primary}
  {secondary}
  {menu}
  {notices}
>
  {#snippet views()}
    {#if bar}{@render bar()}{:else}<PlaceViews placeId={hubId} {tool} {current} {onnavigate} />{/if}
  {/snippet}
</PlaceHead>
