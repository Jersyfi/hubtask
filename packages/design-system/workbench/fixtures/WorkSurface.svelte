<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The work surface's foundation side by side, laid out as the concept's scenes draw it, so a
  // reviewer can hold it beside the mockup crops (docs/backlog/work-surface/mockups): the heads of
  // the scene *grammar*, the entry of *task* with its jump marks, folding sections and rail, the
  // status menu of *wf*, the composer of *deskentry*, the queue, the query line, Today's cards and a
  // product band. Every part is a block of wave 7; the content is the demos'.
  import IconButton from '../../src/IconButton.svelte';
  import JumpMarks from '../../src/JumpMarks.svelte';
  import ObjectHead from '../../src/ObjectHead.svelte';
  import StatusButton from '../../src/StatusButton.svelte';
  import ComposerDemo from '../../src/_ComposerDemo.svelte';
  import FoldingSectionDemo from '../../src/_FoldingSectionDemo.svelte';
  import ObjectHeadDemo from '../../src/_ObjectHeadDemo.svelte';
  import ProductBandDemo from '../../src/_ProductBandDemo.svelte';
  import PropertyRailDemo from '../../src/_PropertyRailDemo.svelte';
  import QueryLineDemo from '../../src/_QueryLineDemo.svelte';
  import StateLozengeDemo from '../../src/_StateLozengeDemo.svelte';
  import StatusButtonDemo from '../../src/_StatusButtonDemo.svelte';
  import WidgetDemo from '../../src/_WidgetDemo.svelte';
  import WorkTable from '../../src/_WorkTable.svelte';
  import { IN_PROGRESS, STORY_STEPS } from '../../src/_workFixtures.ts';

  interface Props {
    /** One part alone, for a crop; absent, every part. */
    part?: 'grammar' | 'entry' | 'status' | 'composer' | 'queue' | 'query' | 'today' | 'band' | 'lozenges';
  }

  const { part }: Props = $props();
  const shows = (name: string) => part === undefined || part === name;
  /** The entry's section in view; on a phone the jump marks are the switch and show only it. */
  let mark = $state('gallery-description');
</script>

{#snippet end()}
  <IconButton icon="star" label="Star" tone="secondary" size="sm" />
  <IconButton icon="link" label="Copy link" tone="secondary" size="sm" />
{/snippet}
{#snippet points()}<span>↑ P2</span><span>8 points</span>{/snippet}

<div class="gallery">
  {#if shows('grammar')}
    <section class="part" data-part="grammar" aria-label="ObjectHead in six products">
      <ObjectHeadDemo mode="grammar" />
    </section>
  {/if}

  {#if shows('entry')}
    <section class="part" data-part="entry" aria-label="An entry: head, jump marks, sections, rail">
      <ObjectHead
        objectKey={{ label: 'TAMA-18', icon: 'square-check' }}
        type={{ label: 'Story', icon: 'square-check' }}
        meta="Backlog · target version 4.3.0 · created 3 Nov by Clara Neumann"
        title="Pay with Apple Pay and Google Pay in the app’s checkout"
        titleLabel="Title"
        editTitleLabel="Edit title"
        onrename={() => {}}
        next={{ label: 'Request review', onclick: () => {} }}
        person={{ name: 'Amelia Hughes' }}
        due={{ label: '3 – 27 Nov' }}
        facts={points}
        {end}
      >
        {#snippet status()}<StatusButton current={IN_PROGRESS} steps={STORY_STEPS} label="Status: In progress. Change status" menuTitle="From “In progress” on" closeLabel="Close" />{/snippet}
      </ObjectHead>
      <div class="entry">
        <div class="main" data-show={mark}>
          <JumpMarks
            bind:current={mark}
            label="Sections of TAMA-18"
            marks={[
              { id: 'gallery-description', label: 'Description' },
              { id: 'gallery-parts', label: 'Parts', count: '2/8' },
              { id: 'gallery-links', label: 'Links', count: '7' },
              { id: 'gallery-files', label: 'Files', count: '9' },
              { id: 'gallery-conversation', label: 'Conversation', count: '21' },
            ]}
          />
          <FoldingSectionDemo prefix="gallery" />
        </div>
        <div class="rail"><PropertyRailDemo mode="story" /></div>
      </div>
    </section>
  {/if}

  {#if shows('status')}
    <section class="part" data-part="status" aria-label="StatusButton">
      <div class="row"><StatusButtonDemo mode="workflow" /><StatusButtonDemo mode="free" /><StatusButtonDemo mode="readonly" /></div>
    </section>
  {/if}

  {#if shows('composer')}
    <section class="part" data-part="composer" aria-label="Composer">
      <div class="row"><ComposerDemo mode="ticket" /><ComposerDemo mode="note" /></div>
      <PropertyRailDemo mode="ticket" />
    </section>
  {/if}

  {#if shows('queue')}
    <section class="part" data-part="queue" aria-label="The queue">
      <WorkTable mode="queue" />
    </section>
  {/if}

  {#if shows('query')}
    <section class="part" data-part="query" aria-label="QueryLine">
      <QueryLineDemo mode="filled" />
      <QueryLineDemo mode="error" />
    </section>
  {/if}

  {#if shows('today')}
    <section class="part" data-part="today" aria-label="Widgets">
      <WidgetDemo mode="today" />
      <WidgetDemo mode="overview" />
    </section>
  {/if}

  {#if shows('band')}
    <section class="part" data-part="band" aria-label="ProductBand">
      <ProductBandDemo mode="desk" />
    </section>
  {/if}

  {#if shows('lozenges')}
    <section class="part" data-part="lozenges" aria-label="StateLozenge">
      <StateLozengeDemo mode="vocabularies" />
    </section>
  {/if}
</div>

<style>
  .gallery { display: flex; flex-direction: column; gap: var(--sp-600); padding: var(--sp-300) var(--sp-400); background: var(--bg-canvas); }
  .part { display: flex; flex-direction: column; gap: var(--sp-300); min-inline-size: 0; }
  .entry { display: grid; grid-template-columns: minmax(0, 1fr) var(--layout-rail); gap: var(--sp-400); align-items: start; }
  .main { display: flex; flex-direction: column; min-inline-size: 0; }
  .row { display: flex; flex-wrap: wrap; gap: var(--sp-400); align-items: flex-start; }
  .row > :global(*) { flex: 1 1 0; min-inline-size: 0; }

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width < 905px) {
    .gallery { padding: var(--sp-200); }
    .entry { grid-template-columns: minmax(0, 1fr); }
  }

  /* The entry on a phone (§7, scene task): *All properties* right under the head, the jump marks
     as the switch between sections, one section shown, the composer docked. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .rail { order: -1; }
    .main[data-show] :global(section[id^='gallery-']) { display: none; }
    .main[data-show='gallery-description'] :global(#gallery-description),
    .main[data-show='gallery-links'] :global(#gallery-links),
    .main[data-show='gallery-conversation'] :global(#gallery-conversation) { display: block; }
  }
</style>
