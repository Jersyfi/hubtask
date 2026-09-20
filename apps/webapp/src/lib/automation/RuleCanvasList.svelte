<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One list of steps on the canvas: the chain, or one arm of a branch (F8-04, decision 1).
  //
  // Recursive, because a branch's arm is a list of steps like the chain is - the self-import is
  // how Svelte 5 recurses. Every gap carries one insertion point; a stop at the end of a list
  // draws no gap after it, because nothing runs after a stop.

  import { Icon, type IconName } from '@hubtask/design-system/components';

  import InsertMenu from './InsertMenu.svelte';
  import RuleCanvasList from './RuleCanvasList.svelte';
  import { DRAG_TYPE, type Drag, type Selection } from './selection.ts';
  import { countSteps, depthOf, type Step } from './model.ts';
  import { conditionWords, kindWord, type Names } from './words.ts';
  import type { Verdict } from './probe.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    steps: readonly Step[];
    /** The list's own path: `''` for the chain, `2/then` for an arm. */
    prefix: string;
    kinds: readonly string[];
    names: Names;
    selection: Selection;
    marks?: ReadonlyMap<string, string>;
    /** One line under a step's title: its settings in words. */
    describe?: (step: Step) => string;
    onselect: (selection: Selection) => void;
    oninsert: (list: string, index: number, kind: string) => void;
    onremove: (path: string) => void;
    onfold: (path: string) => void;
    /** Move one place up or down inside its list - what the drag does, by keyboard (F8-05). */
    onnudge: (path: string, direction: -1 | 1) => void;
    /** The drag in flight, and what happens when it starts, ends, or lands on a gap. */
    drag?: Drag;
    ondragchange: (drag: Drag | undefined) => void;
    ondrop: (list: string, index: number, drag: Drag) => void;
    /** One arm at a time (decision 8): on a narrow screen, and from the second nesting depth. */
    segmented: boolean;
    armChoice: ReadonlyMap<string, 'then' | 'else'>;
    onpickarm: (path: string, arm: 'then' | 'else') => void;
    verdicts?: ReadonlyMap<string, Verdict>;
    dimUnvisited?: boolean;
  }

  const {
    steps, prefix, kinds, names, selection, marks, describe, onselect, oninsert, onremove, onfold,
    onnudge, drag, ondragchange, ondrop, segmented, armChoice, onpickarm, verdicts, dimUnvisited = false,
  }: Props = $props();

  const verdictWord = (verdict: Verdict): string => t(verdict.code, verdict.params);

  /** Whether a card is inert while something is lifted: everything but the lifted card's own subtree. */
  const inert = (path: string): boolean => drag !== undefined && !(drag.src === 'step' && (drag.path === path || path.startsWith(`${drag.path}/`)));

  function liftStep(event: DragEvent, path: string): void {
    if (!event.dataTransfer) return;
    event.dataTransfer.setData(DRAG_TYPE, JSON.stringify({ src: 'step', path }));
    event.dataTransfer.effectAllowed = 'move';
    ondragchange({ src: 'step', path });
  }

  const words = { t, has: (code: string) => messages.has(code) };

  const FLOW_ICON: Record<string, IconName> = { WAIT: 'pause', BRANCH: 'git-branch', STOP: 'square' };

  const pathOf = (index: number): string => (prefix ? `${prefix}/${index}` : String(index));
  const isSelected = (path: string): boolean => selection.kind === 'step' && selection.path === path;

  function iconOf(kind: string): IconName {
    if (FLOW_ICON[kind]) return FLOW_ICON[kind];
    if (kind.startsWith('AI_')) return 'sparkles';
    if (kind.includes('WEBHOOK') || kind === 'HTTP_REQUEST') return 'globe';
    if (kind.includes('LABEL')) return 'tag';
    if (kind.includes('ASSIGN') || kind.includes('MEMBER')) return 'user';
    if (kind.includes('COMMENT')) return 'message-square';
    if (kind.includes('DUE') || kind.includes('RECURRENCE') || kind.includes('OCCURRENCE')) return 'calendar';
    if (kind.includes('NOTIF') || kind.includes('EMAIL')) return 'bell';
    return 'check';
  }

  function meta(step: Step): string {
    if (step.kind === 'BRANCH') return t('app.flow.card_branch_if', { condition: conditionWords(words, names, String(step.params.condition ?? '')) });
    if (step.kind === 'STOP') return t('app.flow.card_stop_hint');
    if (step.kind === 'WAIT') return String(step.params.duration ?? '');
    return describe?.(step) ?? '';
  }

  /** The two arms, in the order they are drawn. */
  const arms = (step: Step): { arm: 'then' | 'else'; list: readonly Step[] }[] => [
    { arm: 'then', list: step.then ?? [] },
    { arm: 'else', list: step.else ?? [] },
  ];

  const endsInStop = (list: readonly Step[] | undefined): boolean => (list?.length ?? 0) > 0 && list?.[list.length - 1]?.kind === 'STOP';

  function onkey(event: KeyboardEvent, select: () => void): void {
    // A key on a tool inside the card is the tool's, not the card's: the card's own handler
    // would otherwise swallow the Enter that presses "move down".
    if (event.target !== event.currentTarget) return;
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      select();
    }
  }
</script>

{#each steps as step, index (pathOf(index))}
  {@const path = pathOf(index)}
  {@const isFlow = step.kind in FLOW_ICON}
  {@const verdict = verdicts?.get(path)}
  <div
    class="card"
    class:stop={step.kind === 'STOP'}
    class:selected={isSelected(path)}
    class:inert={inert(path)}
    class:lit={verdict !== undefined && verdict.state !== 'skipped'}
    class:yes={verdict?.state === 'yes'}
    class:no={verdict?.state === 'no'}
    class:faded={(dimUnvisited && verdict === undefined) || verdict?.state === 'skipped'}
    class:lifted={drag?.src === 'step' && drag.path === path}
    data-card={path}
    data-depth={depthOf(prefix)}
    role="button"
    tabindex="0"
    draggable="true"
    onclick={() => onselect({ kind: 'step', path })}
    onkeydown={(event) => onkey(event, () => onselect({ kind: 'step', path }))}
    ondragstart={(event) => liftStep(event, path)}
    ondragend={() => ondragchange(undefined)}
  >
    <span class="mark" class:flow={isFlow} class:ai={step.kind.startsWith('AI_')}><Icon name={iconOf(step.kind)} size="sm" /></span>
    <span class="body">
      <span class="kind">{step.kind === 'BRANCH' ? t('app.flow.card_branch') : isFlow ? t('app.flow.card_flow') : t('app.flow.card_action')}</span>
      <span class="title">{kindWord(words, step.kind)}</span>
      {#if meta(step)}<span class="meta">{meta(step)}</span>{/if}
      {#if marks?.get(path)}<span class="flag"><Icon name="triangle-alert" size="sm" />{marks.get(path)}</span>{/if}
    </span>
    {#if verdict}<span class="verdict" class:yes={verdict.state === 'yes'} class:no={verdict.state === 'no'}><Icon name={verdict.state === 'yes' ? 'check' : 'x'} size="sm" />{verdictWord(verdict)}</span>{/if}
    <span class="tools">
      <button
        class="tool"
        type="button"
        aria-label={t('app.flow.card_move_up')}
        disabled={index === 0}
        onclick={(event) => {
          event.stopPropagation();
          onnudge(path, -1);
        }}
      >
        <Icon name="chevron-up" size="sm" />
      </button>
      <button
        class="tool"
        type="button"
        aria-label={t('app.flow.card_move_down')}
        disabled={index === steps.length - 1}
        onclick={(event) => {
          event.stopPropagation();
          onnudge(path, 1);
        }}
      >
        <Icon name="chevron-down" size="sm" />
      </button>
      {#if step.kind === 'BRANCH'}
        <button
          class="tool"
          type="button"
          aria-label={step.collapsed ? t('app.flow.card_unfold') : t('app.flow.card_fold')}
          onclick={(event) => {
            event.stopPropagation();
            onfold(path);
          }}
        >
          <Icon name="fold-vertical" size="sm" />
        </button>
      {/if}
      <button
        class="tool"
        type="button"
        aria-label={t('app.flow.card_remove')}
        onclick={(event) => {
          event.stopPropagation();
          onremove(path);
        }}
      >
        <Icon name="trash" size="sm" />
      </button>
    </span>
  </div>

  {#if step.kind === 'BRANCH'}
    <span class="stub"></span>
    {#if step.collapsed}
      <button class="folded" type="button" onclick={() => onfold(path)}>
        <Icon name="git-branch" size="sm" />
        <span>
          {t('app.flow.card_folded', { then: countSteps(step.then ?? []), else: countSteps(step.else ?? []) })}
        </span>
      </button>
    {:else}
      {@const seg = segmented || depthOf(`${path}/then`) >= 2}
      {@const shown = armChoice.get(path) ?? 'then'}
      <div class="arms" class:seg data-branch={path}>
        {#if seg}
          <div class="seghead" role="tablist">
            {#each arms(step) as { arm, list } (arm)}
              <button type="button" role="tab" aria-selected={shown === arm} data-arm-pick={`${path}/${arm}`} onclick={() => onpickarm(path, arm)}>
                <Icon name={arm === 'then' ? 'check' : 'x'} size="sm" />{arm === 'then' ? t('app.flow.card_then') : t('app.flow.card_otherwise')} ({countSteps(list)})
              </button>
            {/each}
          </div>
        {/if}
        {#each arms(step) as { arm, list } (arm)}
          {#if !seg || shown === arm}
            <div class="arm" data-arm={`${path}/${arm}`}>
              <span class="stub"></span>
              {#if !seg}<span class="armlabel"><Icon name={arm === 'then' ? 'check' : 'x'} size="sm" />{arm === 'then' ? t('app.flow.card_then') : t('app.flow.card_otherwise')}</span>{/if}
              {#if list.length === 0}
                <span class="empty">{t('app.flow.card_arm_empty')}</span>
                <InsertMenu {kinds} list={`${path}/${arm}`} index={0} onpick={oninsert} {drag} {ondrop} />
              {:else}
                <RuleCanvasList steps={list} prefix={`${path}/${arm}`} {kinds} {names} {selection} {marks} {describe} {onselect} {oninsert} {onremove} {onfold} {onnudge} {drag} {ondragchange} {ondrop} {segmented} {armChoice} {onpickarm} {verdicts} {dimUnvisited} />
              {/if}
              {#if !endsInStop(list)}<span class="tail"></span>{/if}
            </div>
          {/if}
        {/each}
        {#if !seg}<span class="join"></span>{/if}
      </div>
    {/if}
    <span class="stub"></span>
  {/if}

  {#if step.kind !== 'STOP' || index < steps.length - 1}
    <InsertMenu
      {kinds}
      list={prefix}
      index={index + 1}
      onpick={oninsert}
      {drag}
      {ondrop}
      caption={step.kind === 'WAIT'
        ? t('app.flow.card_wait_later', { duration: String(step.params.duration ?? '') })
        : step.kind === 'BRANCH'
          ? t('app.flow.card_after_branch')
          : undefined}
    />
  {/if}
{/each}

<style>
  .card {
    position: relative;
    width: min(44ch, 100%);
    display: flex;
    gap: var(--sp-150);
    align-items: flex-start;
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
    text-align: start;
    cursor: pointer;
  }

  .card.stop { width: min(28ch, 100%); border-style: dashed; box-shadow: none; background: var(--bg-surface-sunken); }

  .card.selected { outline: var(--bw-ring) solid var(--accent-primary); outline-offset: var(--sp-025); }

  .card:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .body { flex: 1 1 auto; min-width: 0; display: flex; flex-direction: column; gap: var(--sp-025); }

  .kind { font-size: var(--fs-050); font-weight: var(--fw-medium); text-transform: uppercase; color: var(--text-subtle); }

  .title { font-weight: var(--fw-medium); color: var(--text-primary); overflow-wrap: anywhere; }

  .meta { font-size: var(--fs-075); color: var(--text-secondary); overflow-wrap: anywhere; }

  .flag { display: inline-flex; align-items: center; gap: var(--sp-050); font-size: var(--fs-075); color: var(--text-warning); }

  .mark { flex: 0 0 auto; display: inline-grid; place-items: center; width: var(--sp-300); height: var(--sp-300); border-radius: var(--r-sm); background: var(--label-blue-bg); color: var(--label-blue-fg); }

  .mark.flow { background: var(--label-violet-bg); color: var(--label-violet-fg); }

  .mark.ai { background: var(--label-green-bg); color: var(--label-green-fg); }

  /* The tools appear on hover, focus, or selection: a card is read more often than edited. */
  .tools { display: flex; flex-direction: column; gap: var(--sp-025); opacity: 0; transition: opacity var(--motion-state-duration) var(--motion-state-easing); }

  .card:hover .tools, .card.selected .tools, .card:focus-within .tools { opacity: 1; }

  .tool { width: var(--sp-300); height: var(--sp-300); padding: 0; display: grid; place-items: center; border: 0; border-radius: var(--r-xs); background: transparent; color: var(--text-subtle); }

  .tool:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  .tool:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }

  .tool[disabled] { opacity: 0.35; cursor: default; }

  /* While a piece is lifted: the card being moved and its subtree stay, everything else fades. */
  .card.inert { opacity: 0.35; }

  .card.lifted { opacity: 0.6; }

  /* A run drawn on: a lit card carries its word; a skipped or never-reached one fades. */
  .card.lit { box-shadow: 0 0 0 var(--sp-050) var(--accent-primary-subtle), var(--shadow-raised); }

  .card.lit.yes { border-color: var(--success-500); }

  .card.lit.no { border-color: var(--warning-500); }

  .card.faded { opacity: 0.38; }

  .verdict { position: absolute; inset-block-start: calc(-1 * var(--sp-150)); inset-inline-end: var(--sp-150); display: inline-flex; align-items: center; gap: var(--sp-050); padding: 0 var(--sp-100); min-height: var(--sp-250); border-radius: var(--r-full); font-size: var(--fs-050); font-weight: var(--fw-medium); background: var(--label-slate-bg); color: var(--label-slate-fg); animation: arrive var(--motion-entrance-duration) var(--motion-entrance-easing) both; }

  .verdict.yes { background: var(--label-green-bg); color: var(--label-green-fg); }

  .verdict.no { background: var(--label-amber-bg); color: var(--label-amber-fg); }

  @keyframes arrive { from { opacity: 0; translate: 0 var(--sp-050); } to { opacity: 1; translate: none; } }

  @media (prefers-reduced-motion: reduce) { .verdict { animation: none; } }
  :global([data-motion='reduced']) .verdict { animation: none; }

  /* A card is a thing to move, not text to select: a selection that began on its title would
     otherwise be what the browser drags. */
  .card { cursor: grab; user-select: none; }

  /* One arm at a time: the fork's bars are gone, a switch names both arms with their counts. */
  .arms.seg { display: flex; flex-direction: column; align-items: center; }

  .arms.seg::before { display: none; }

  .seghead { display: inline-flex; margin-block: var(--sp-050); border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-full); overflow: hidden; background: var(--bg-surface); }

  .seghead button { display: inline-flex; align-items: center; gap: var(--sp-050); padding: var(--sp-050) var(--sp-150); border: 0; background: transparent; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .seghead button[aria-selected='true'] { background: var(--label-violet-bg); color: var(--label-violet-fg); }

  .seghead button:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--sp-025)); }

  .arms.seg .arm { width: 100%; }

  .stub, .tail { width: var(--bw-ring); height: var(--sp-150); background: var(--border-default); border-radius: var(--r-full); flex: 0 0 auto; }

  .tail { flex: 1 1 auto; min-height: var(--sp-150); }

  /* The fork: a bar across the arms' centres, one column each, equal in height so both tails
     reach the join. `grid-column: 1 / -1` on the join, or it would sit in the first column. */
  .arms {
    position: relative;
    display: grid;
    grid-template-columns: 1fr 1fr;
    column-gap: var(--sp-300);
    width: min(92ch, 100%);
  }

  .arms::before {
    content: '';
    position: absolute;
    inset-block-start: 0;
    inset-inline: calc(25% - var(--sp-300) / 4);
    height: var(--bw-ring);
    background: var(--border-default);
  }

  .arm { display: flex; flex-direction: column; align-items: center; min-width: 0; }

  .arm .empty { width: 100%; }

  .arm :global(.card) { width: 100%; }

  .armlabel {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    margin-block: var(--sp-050);
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-full);
    background: var(--label-violet-bg);
    color: var(--label-violet-fg);
    font-size: var(--fs-050);
    font-weight: var(--fw-medium);
   
    text-transform: uppercase;
  }

  .empty { padding: var(--sp-100); border: var(--bw-hairline) dashed var(--border-default); border-radius: var(--r-md); text-align: center; font-size: var(--fs-075); color: var(--text-subtle); }

  .join { grid-column: 1 / -1; position: relative; height: var(--bw-ring); }

  .join::before { content: ''; position: absolute; inset-block: 0; inset-inline: calc(25% - var(--sp-300) / 4); background: var(--border-default); }

  .folded {
    width: min(44ch, 100%);
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) dashed var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font-size: var(--fs-075);
    text-align: start;
  }

  .folded:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }


  @media (prefers-reduced-motion: reduce) { .tools { transition: none; } }
  :global([data-motion='reduced']) .tools { transition: none; }
</style>
