<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one head of a hub tool's tabs (the navigation concept §2.4, §3): the trail to the hub, the
  // tool's name - a queue's on *Queues*, with its ☆ -, one figure slot, the tab's actions, then the
  // tool's bar at the offset a collection's stands at (`PlaceHead`), so switching *Schedule*,
  // *Releases* and *Sprints* moves neither the title nor the bar.
  import type { Snippet } from 'svelte';
  import { Icon, type MenuItem, type PageAction } from '@hubtask/design-system/components';

  import { containers } from '../data/containers.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
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
  const row = $derived(HUB_TOOLS.find((each) => each.id === tool));
  const name = $derived(t(row?.code ?? ''));
  // The inventory's head carries the tool's tile before its name (scene invlist).
  const hasMark = $derived(tool === 'inventory' && row !== undefined);
  const trail = $derived([
    ...(hub ? [{ id: hub.id, label: hub.name, href: `/hubs/${hub.id}` }] : []),
    { id: tool, label: name },
  ]);
</script>

<!-- On a phone the title stands in the bar and the figure alone fills the head's row: it keeps to
     that row's one line, so a long sentence never moves the tool's bar from tab to tab. -->
{#snippet toolMark()}<span class="tool-mark" data-tool-mark={tool}><Icon name={row?.icon ?? 'shield'} size="md" /></span>{/snippet}
{#snippet oneLine()}<span class="figure" data-compact={viewport.isCompact ? '' : undefined}>{@render figure?.()}</span>{/snippet}
<PlaceHead
  kind="tool"
  title={title ?? name}
  {trail}
  trailLabel={t('app.product_home.trail')}
  expandLabel={t('app.admin.expand_trail')}
  oncrumb={(crumbId) => onnavigate(crumbId === hubId ? `/hubs/${hubId}` : toolPath(hubId, tool))}
  mark={hasMark ? toolMark : undefined}
  {titleEnd}
  figure={figure ? oneLine : undefined}
  {primary}
  {secondary}
  {menu}
  {notices}
>
  {#snippet views()}
    {#if bar}{@render bar()}{:else}<PlaceViews placeId={hubId} {tool} {current} {onnavigate} />{/if}
  {/snippet}
</PlaceHead>

<style>
  .figure { min-width: 0; }
  .tool-mark {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    inline-size: var(--sp-400);
    block-size: var(--sp-400);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
  }
  .figure[data-compact] {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
</style>
