<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The navigation concept's mockup (docs/backlog/navigation/mockup.html, §4) rebuilt from the
  // wave-5 pieces, with its own data: the workspace level, a product team's hub level, an IT hub
  // with queues, the content tree that drags, and the rail.
  import Button from './Button.svelte';
  import NavFlyout from './NavFlyout.svelte';
  import NavGroup, { type NavTreeNode } from './NavGroup.svelte';
  import NavItem from './NavItem.svelte';
  import NavLevel from './NavLevel.svelte';
  import Toast from './Toast.svelte';
  import { applyPlacement, positionOf, type Placement } from './navtree.ts';

  type Mode = 'workspace' | 'hub' | 'queue' | 'wiki' | 'drag' | 'rail' | 'empty';
  const { mode = 'hub' }: { mode?: Mode } = $props();
  let isMoreOpen = $state(true);
  const hide = (name: string) => ({ label: `Manage ${name}`, items: [{ id: 'hide', label: 'Hide from the sidebar', icon: 'eye-off' as const }], onselect: () => {} });

  const manage = (name: string) => ({
    label: `Manage ${name}`,
    items: [
      { id: 'star', label: 'Star' },
      { id: 'move', label: 'Move to…' },
      { id: 'archive', label: 'Archive' },
    ],
    onselect: () => {},
  });
  const createUnder = (name: string) => ({
    label: `Create under ${name}`,
    items: [
      { id: 'page', label: 'Page' },
      { id: 'template', label: 'From a template…' },
      { id: 'board', label: 'Whiteboard' },
    ],
    onselect: () => {},
  });

  const page = (id: string, label: string, children?: NavTreeNode[]): NavTreeNode => ({
    id,
    label,
    icon: 'file-text',
    kind: 'page',
    part: 'contents',
    children,
  });
  const board = (id: string, label: string): NavTreeNode => ({
    id,
    label,
    icon: 'layout-template',
    kind: 'whiteboard',
    part: 'contents',
    canHoldChildren: false,
  });

  let contents = $state<NavTreeNode[]>([
    page('start', 'Start'),
    page('team', 'Team', [page('onboarding', 'Onboarding handbook'), page('how', 'How we work'), board('retro', 'Retro sprint 41'), page('holiday', 'Holiday rules')]),
    page('architecture', 'Architecture', [page('adr', 'Decisions')]),
    page('product', 'Product', [page('vision', 'Vision')]),
    page('notes', 'Release notes', [page('n32', '3.2')]),
    board('workshop', 'Roadmap workshop'),
    board('storymap', 'Story map checkout'),
    ...Array.from({ length: 24 }, (_, i) => page(`p${i}`, `Meeting notes ${i + 1}`)),
  ]);
  let expanded = $state(['team']);
  let filter = $state('');
  let kind = $state('all');
  let landed = $state<string | undefined>(undefined);
  let undo = $state<{ text: string; before: NavTreeNode[] } | null>(null);
  let heard = $state('');

  const collections: NavTreeNode[] = [
    { id: 'web', label: 'Web app', icon: 'collection', color: 'blue', part: 'collections', canHoldChildren: false, figure: '6 d', badge: { icon: 'repeat', label: 'Runs sprints' } },
    { id: 'mobile', label: 'Mobile', icon: 'collection', color: 'blue', part: 'collections', canHoldChildren: false, figure: '9 d', badge: { icon: 'repeat', label: 'Runs sprints' } },
    { id: 'design', label: 'Design', icon: 'collection', color: 'blue', part: 'collections', canHoldChildren: false, figure: '14' },
    { id: 'bugs', label: 'Bugs', icon: 'collection', color: 'blue', part: 'collections', canHoldChildren: false, figure: '3 overdue', isFigureUrgent: true },
  ];
  let collectionOrder = $state(collections);


  function place(id: string, placement: Placement) {
    const before = contents;
    contents = applyPlacement(contents, id, placement);
    if (placement.parentId && !expanded.includes(placement.parentId)) expanded = [...expanded, placement.parentId];
    landed = id;
    const at = positionOf(contents, id);
    heard = at ? `Position ${at.position} of ${at.of}${at.parent ? ` under ${at.parent.label}` : ''}` : '';
    undo = { text: 'Moved', before };
  }

  const current = $derived(mode === 'wiki' ? 'onboarding' : mode === 'queue' ? 'q-breach' : 'overview');
  const levelKey = $derived(mode === 'workspace' || mode === 'rail' ? 'workspace' : mode === 'queue' ? 'it' : 'pe');
</script>

<div class="frame" data-rail={mode === 'rail' ? '' : undefined}>
  <aside class="side" aria-label="Navigation">
    <ul class="foryou">
      <NavItem label="Today" icon="sun" href="#today" figure="5" isCurrent={mode === 'workspace'} isRail={mode === 'rail'} />
      <NavItem label="Inbox" icon="jumble" href="#inbox" figure="3" isRail={mode === 'rail'} />
      <NavItem label="Waiting for your yes" icon="user-check" href="#yes" figure="2" isFigureUrgent isRail={mode === 'rail'} />
    </ul>

    {#if mode === 'rail'}
      <ul class="foryou">
        <NavItem label="Recent" icon="rotate-ccw" href="#recent" isRail />
        <NavItem label="Plans" icon="calendar" color="orange" href="#plans" isRail />
        <NavItem label="Pages" icon="book-open" color="teal" href="#pages" isRail />
        <NavItem label="Product development" icon="hub" color="orange" href="#pe" isRail />
        <NavItem label="IT operations" icon="hub" color="teal" href="#it" isRail />
      </ul>
      <div class="grow"></div>
    {:else if mode === 'workspace'}
      <NavLevel levelKey={levelKey} label="Workspace">
        <ul class="plain">
          <li>
            <NavFlyout label="Recent" trigger="row" icon="rotate-ccw" triggerLabel="Recent">
              <ul class="plain">
                <NavItem label="Web app · Board" icon="collection" color="blue" href="#web" />
                <NavItem label="Onboarding handbook" icon="book-open" color="teal" href="#onb" />
                <NavItem label="About to breach · IT operations" icon="inbox" color="magenta" href="#q" />
              </ul>
            </NavFlyout>
          </li>
        </ul>
        <NavGroup label="Starred" collapseLabel="Fold Starred">
          <NavItem label="Web app" icon="collection" color="blue" href="#web" figure="Sprint 42" isMovable menu={manage('Web app')} />
          <NavItem label="About to breach · IT" icon="columns-3" href="#q" figure="1" isFigureUrgent isMovable menu={manage('About to breach')} />
          <NavItem label="Onboarding handbook" icon="book-open" color="teal" href="#onb" isMovable menu={manage('Onboarding handbook')} />
        </NavGroup>
        <NavGroup
          label="Products"
          collapseLabel="Fold Products"
          hiddenLabel="More"
          hiddenRows={[
            { id: 'time', label: 'Time', icon: 'clock', color: 'green', href: '#time' },
            { id: 'goals', label: 'Goals', icon: 'flag', color: 'violet', href: '#goals' },
          ]}
          rowMenu={(id) => ({ label: `Manage ${id}`, items: [{ id: 'show', label: 'Show in the sidebar', icon: 'eye' }], onselect: () => {} })}
          bind:isHiddenOpen={isMoreOpen}
        >
          <NavItem label="Plans" icon="calendar" color="orange" href="#plans" menu={hide('Plans')} />
          <NavItem label="Pages" icon="book-open" color="teal" href="#pages" menu={hide('Pages')} />
          <NavItem label="Whiteboards" icon="layout-template" color="amber" href="#boards" menu={hide('Whiteboards')} />
          <NavItem label="Service desk" icon="inbox" color="magenta" href="#desk" figure="2" isFigureUrgent menu={hide('Service desk')} />
          <NavItem label="Assets" icon="shield" color="slate" href="#assets" menu={hide('Assets')} />
        </NavGroup>
        <NavGroup label="Hubs" collapseLabel="Fold Hubs" add={{ label: 'New hub', onselect: () => {} }} menu={{ label: 'Order hubs', onselect: () => {} }}>
          {#each [['Product development', 'orange'], ['IT operations', 'teal'], ['Marketing', 'magenta'], ['People', 'green'], ['Finance', 'blue'], ['Facilities', 'violet'], ['Sales', 'amber'], ['Purchasing', 'slate']] as [name, color] (name)}
            <NavItem label={name!} icon="hub" {color} href="#hub" figure="›" isMovable menu={manage(name!)} add={{ label: `New collection in ${name}`, onselect: () => {} }} />
          {/each}
          <NavItem label="10 more hubs" icon="ellipsis" tone="more" />
        </NavGroup>
      </NavLevel>
    {:else}
      <NavLevel
        {levelKey}
        label={mode === 'queue' ? 'IT operations' : 'Product development'}
        hub={{
          name: mode === 'queue' ? 'IT operations' : 'Product development',
          icon: 'hub',
          color: mode === 'queue' ? 'teal' : 'orange',
          backLabel: 'All hubs',
          backHref: '#hubs',
          starLabel: 'Star this hub',
          menu: { label: 'Manage this hub', items: [{ id: 'settings', label: 'Settings…' }, { id: 'products', label: 'Products' }], onselect: () => {} },
        }}
      >
        {#snippet switcher()}
          <NavFlyout label="Switch hub" trigger="icon" icon="chevrons-up-down" triggerLabel="Switch hub" searchLabel="Find a hub">
            <NavGroup label="Starred">
              <NavItem label="IT operations" icon="hub" color="teal" href="#it" />
            </NavGroup>
            <NavGroup label="Recently visited">
              <NavItem label="Marketing" icon="hub" color="magenta" href="#mk" />
              <NavItem label="People" icon="hub" color="green" href="#pp" />
            </NavGroup>
            <ul class="plain"><NavItem label="All hubs" icon="hub" tone="link" href="#hubs" /></ul>
          </NavFlyout>
        {/snippet}
        {#snippet tools()}
          <NavItem label="Overview" icon="hub" href="#overview" isCurrent={current === 'overview'} />
          {#if mode === 'queue'}
            <NavItem label="Queues" icon="inbox" twist="expanded" twistLabel="Fold queues" href="#queues" />
            <NavItem label="About to breach" icon="star" level={1} href="#q1" figure="1" isFigureUrgent isMovable isCurrent menu={manage('About to breach')} />
            <NavItem label="Mine" icon="columns-3" level={1} href="#q2" figure="3" isMovable menu={manage('Mine')} />
            <NavItem label="Unassigned" icon="columns-3" level={1} href="#q3" figure="2" isMovable menu={manage('Unassigned')} />
            <NavItem label="Unhappy" icon="columns-3" level={1} href="#q4" figure="0" isMovable menu={manage('Unhappy')} />
            <NavItem label="All open" icon="columns-3" level={1} href="#q5" figure="14" isMovable menu={manage('All open')} />
            <NavItem label="Inventory" icon="shield" href="#inv" figure="6 !" isFigureUrgent />
            <NavItem label="Goals" icon="flag" href="#goals" />
          {:else}
            <NavItem label="Plan" icon="calendar" href="#plan" figure="Sprint 42 · 6 d" />
            <NavItem label="Time" icon="clock" href="#time" />
            <NavItem label="Goals" icon="flag" href="#goals" figure="1 at risk" />
          {/if}
        {/snippet}
        {#if mode === 'queue'}
          <NavGroup label="Collections" collapseLabel="Fold collections" add={{ label: 'New collection', onselect: () => {} }} menu={{ label: 'Collections', onselect: () => {} }}>
            <NavItem label="Service desk" icon="inbox" color="magenta" href="#sd" figure="1 SLA" isFigureUrgent isMovable menu={manage('Service desk')} add={createUnder('Service desk')} />
            <NavItem label="Changes" icon="collection" color="blue" href="#ch" figure="4" isMovable menu={manage('Changes')} />
            <NavItem label="Problems" icon="collection" color="blue" href="#pr" figure="2" isMovable menu={manage('Problems')} />
          </NavGroup>
          <NavGroup
            label="Contents"
            count="46"
            collapseLabel="Fold contents"
            add={createUnder('Contents')}
            menu={{ label: 'Contents', onselect: () => {} }}
            tree={[page('desk', 'How we work with this desk'), page('kb', 'Knowledge base', [page('vpn', 'Set up the VPN'), page('printer', 'Connect a printer'), page('pw', 'Reset a password')]), page('runbooks', 'Runbooks', [page('r1', 'Restart')]), board('map', 'Network map')]}
            expanded={['kb']}
            twistLabel={(name, isOpen) => `${isOpen ? 'Close' : 'Open'} ${name}`}
            rowMenu={(id) => manage(id)}
            canOrder={() => true}
            onplace={() => {}}
          />
        {:else if mode === 'empty'}
          <NavGroup label="Collections" add={{ label: 'New collection', onselect: () => {} }}>
            <NavItem label="To do" icon="collection" color="violet" href="#todo" figure="7" />
            <NavItem label="Shopping" icon="collection" color="violet" href="#shop" figure="4" />
          </NavGroup>
          <NavGroup label="Contents" add={createUnder('Contents')}>
            <NavItem label="Create the first page" icon="plus" tone="link" level={1} />
          </NavGroup>
        {:else}
          <NavGroup
            label="Collections"
            collapseLabel="Fold collections"
            add={{ label: 'New collection', onselect: () => {} }}
            menu={{ label: 'Collections', onselect: () => {} }}
            tree={collectionOrder}
            treeLabel="Collections of Product development"
            rowMenu={(id) => manage(id)}
            rowAdd={(id) => ({ label: `New entry in ${id}`, onselect: () => {} })}
            canOrder={() => mode === 'drag'}
            onplace={(id, placement) => (collectionOrder = applyPlacement(collectionOrder, id, placement))}
            {refusalLabel}
          />
          <NavGroup
            label="Contents"
            count="131"
            collapseLabel="Fold contents"
            add={createUnder('Contents')}
            menu={{ label: 'Contents', items: [{ id: 'tree', label: 'Tree' }, { id: 'recent', label: 'Recently changed' }, { id: 'az', label: 'A–Z' }, { id: 'all', label: 'All contents' }], onselect: () => {} }}
            tree={contents}
            treeLabel="Contents of Product development"
            bind:expanded
            current={current}
            twistLabel={(name, isOpen) => `${isOpen ? 'Close' : 'Open'} ${name}`}
            rowMenu={(id) => manage(id)}
            rowAdd={(id) => (id === 'retro' || id === 'workshop' || id === 'storymap' ? undefined : createUnder(id))}
            canOrder={() => true}
            onplace={place}
            oncancel={() => (heard = 'Cancelled. Nothing changed.')}
            {refusalLabel}
            {landed}
            limit={15}
            moreLabel={(hidden) => `${hidden} more`}
            filterFrom={30}
            filterLabel="Filter contents"
            bind:filter
            kinds={[{ id: 'all', label: 'All' }, { id: 'page', label: 'Pages' }, { id: 'whiteboard', label: 'Whiteboards' }]}
            bind:kind
            kindLabel="Show"
          />
          <ul class="plain"><NavItem label="All contents" icon="file-text" tone="link" level={1} href="#all" /></ul>
        {/if}
      </NavLevel>
    {/if}

    <ul class="foot">
      <NavItem label="Administration" icon="settings" href="#admin" isRail={mode === 'rail'} />
      <NavItem label="Archive" icon="archive" href="#archive" isRail={mode === 'rail'} />
      <NavItem label="Trash" icon="trash" href="#trash" isRail={mode === 'rail'} />
    </ul>
  </aside>

  <div class="aside-note">
    <p class="heard" aria-live="polite">{heard}</p>
    {#if undo}
      <Toast dismissLabel="Dismiss" onDismiss={() => (undo = null)}>
        {#snippet action()}
          <Button size="sm" tone="subtle" onclick={() => { if (undo) contents = undo.before; undo = null; landed = undefined; }}>Undo</Button>
        {/snippet}
        {undo.text}
      </Toast>
    {/if}
  </div>
</div>

<script lang="ts" module>
  function refusalLabel(refusal: 'self' | 'no-children' | 'other-part' | 'too-deep'): string {
    return {
      self: 'Not under itself',
      'no-children': 'A whiteboard has no subpages',
      'other-part': 'Pages stay in Contents',
      'too-deep': 'No deeper than twelve levels',
    }[refusal];
  }
</script>

<style>
  .frame {
    display: flex;
    gap: var(--sp-300);
    align-items: flex-start;
  }

  .side {
    display: flex;
    flex-direction: column;
    inline-size: var(--layout-sidenav-width);
    block-size: var(--layout-panel-width);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    overflow: hidden;
  }

  [data-rail] .side { inline-size: var(--layout-sidenav-rail); }

  .foryou,
  .foot,
  .plain {
    margin: 0;
    padding: calc(var(--sp-050) + var(--sp-025)) var(--sp-100);
    list-style: none;
  }

  .plain { padding: 0; }
  .foryou { border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .foot { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .grow { flex: 1; }

  .aside-note { display: grid; gap: var(--sp-150); max-inline-size: 40ch; }
  .heard { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }
</style>
