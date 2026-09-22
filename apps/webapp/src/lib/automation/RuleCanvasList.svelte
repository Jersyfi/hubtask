<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One list of steps on the canvas: the chain, or one arm of a branch (F8-04, decision 1).
  //
  // Recursive, because a branch's arm is a list of steps like the chain is - the self-import is
  // how Svelte 5 recurses. Every gap carries one insertion point; a step that ends the run on
  // every path draws no gap after it but the end mark, because nothing runs after it
  // (decision 19). A branch whose else arm holds only another branch is a ladder - if, else if,
  // else - and is drawn as rungs rather than as arms inside arms; *+ Else if* under a branch
  // appends one.

  import { Icon, type IconName } from '@hubtask/design-system/components';

  import ConditionWords from './ConditionWords.svelte';
  import InsertMenu from './InsertMenu.svelte';
  import RuleCanvasList from './RuleCanvasList.svelte';
  import { DRAG_TYPE, type Drag, type Selection } from './selection.ts';
  import { countSteps, depthOf, endsAllPaths, endsRun, isRung, rungsOf, unreachableFrom, type Step } from './model.ts';
  import { kindIcon, kindWord, type Names } from './words.ts';
  import type { Verdict } from './probe.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    steps: readonly Step[];
    /** The whole chain, for the gaps to know what they may take (decision 14). */
    actions: readonly Step[];
    /** The list's own path: `''` for the chain, `2/then` for an arm. */
    prefix: string;
    kinds: readonly string[];
    /** Each kind's sentence and how often the workspace uses it, for the `+` popover's list. */
    summaries?: Readonly<Record<string, string>>;
    usage?: ReadonlyMap<string, number>;
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
    /** Append a rung - an *else if* - under the ladder that starts at the branch (decision 19). */
    onaddrung: (path: string) => void;
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
    steps, actions, prefix, kinds, summaries = {}, usage = new Map(), names, selection, marks, describe, onselect, oninsert, onremove, onfold,
    onnudge, onaddrung, drag, ondragchange, ondrop, segmented, armChoice, onpickarm, verdicts, dimUnvisited = false,
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

  const iconOf = (kind: string): IconName => kindIcon(kind);

  function meta(step: Step): string {
    if (step.kind === 'STOP') return t('app.flow.card_stop_hint');
    if (step.kind === 'WAIT') return String(step.params.duration ?? '');
    return describe?.(step) ?? '';
  }

  /** The two arms, in the order they are drawn. */
  const arms = (step: Step): { arm: 'then' | 'else'; list: readonly Step[] }[] => [
    { arm: 'then', list: step.then ?? [] },
    { arm: 'else', list: step.else ?? [] },
  ];

  /** Whether an arm continues to the join: it does unless it ends the run on every path. */
  const continues = (list: readonly Step[] | undefined): boolean => !endsRun(list ?? []);

  /** The join under a fork: whole, one half with its corner, or none - never two lines over each other. */
  const joinOf = (step: Step): 'both' | 'l' | 'r' | 'none' => {
    const left = continues(step.then);
    const right = continues(step.else);
    return left && right ? 'both' : left ? 'l' : right ? 'r' : 'none';
  };

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

{#if prefix !== '' && steps.length > 0}
  <!-- A gap before an arm's first card too, so that a card can be dropped first without a
       second move (the final check of F8-20); the chain's own leading gap is the canvas's. -->
  <InsertMenu {kinds} {summaries} {usage} {actions} list={prefix} index={0} onpick={oninsert} {drag} {ondrop} />
{/if}
{#each steps as step, index (pathOf(index))}
  {@const path = pathOf(index)}
  {@const isFlow = step.kind in FLOW_ICON}
  {@const verdict = verdicts?.get(path)}
  {@const unreachable = unreachableFrom(steps) !== -1 && index >= unreachableFrom(steps)}
  {#if unreachable && index === unreachableFrom(steps)}
    <!-- A stored rule may hold steps after a stop (the server accepts one); the run never reaches
         them, and the canvas says so once rather than drawing them as though it did (decision 14). -->
    <span class="never" role="note">{t('app.flow.card_never_reached')}</span>
  {/if}
  <div
    class="card"
    class:stop={step.kind === 'STOP'}
    class:unreachable
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
      {#if step.kind === 'BRANCH'}
        <span class="cond"><span class="cmark"><Icon name="funnel" size="sm" /></span><ConditionWords expr={String(step.params.condition ?? '')} {names} /></span>
      {:else if meta(step)}<span class="meta">{meta(step)}</span>{/if}
      {#if marks?.get(path)}<span class="flag"><Icon name="triangle-alert" size="sm" />{marks.get(path)}</span>{/if}
    </span>
    {#if verdict}<span class="verdict" class:yes={verdict.state === 'yes'} class:no={verdict.state === 'no'}><Icon name={verdict.state === 'yes' ? 'check' : 'x'} size="sm" />{verdictWord(verdict)}</span>{/if}
    <span class="tools">
      <button
        class="tool"
        type="button"
        aria-label={t('app.flow.card_move_up')}
        disabled={index === 0 || endsAllPaths(step) || endsAllPaths(steps[index - 1])}
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
        disabled={index === steps.length - 1 || endsAllPaths(step) || endsAllPaths(steps[index + 1])}
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
    {#if step.collapsed}
      <span class="stub"></span>
      <button class="folded" type="button" onclick={() => onfold(path)}>
        <Icon name="git-branch" size="sm" />
        <span>
          {t('app.flow.card_folded', { then: countSteps(step.then ?? []), else: countSteps(step.else ?? []) })}
        </span>
      </button>
    {:else if isRung(step)}
      <!-- The ladder: if / else if / … / else. Every rung but the first is the branch that is the
           sole step of the previous else arm; its condition is a card of its own, selected like
           any card and edited in the panel (decision 19). -->
      {@const rungs = rungsOf(step, path)}
      {@const last = rungs[rungs.length - 1]!}
      <span class="stub"></span>
      <div class="ladder" data-ladder={path}>
        {#each rungs as rung, at (rung.path)}
          <div class="rung" data-rung={rung.path}>
            <div class="rhead">
              <span class="rpill">{at === 0 ? t('app.flow.card_then') : t('app.flow.card_else_if')}</span>
              {#if at > 0}
                <button
                  class="rcond"
                  class:selected={isSelected(rung.path)}
                  class:flagged={marks?.has(rung.path)}
                  type="button"
                  data-card={rung.path}
                  onclick={() => onselect({ kind: 'step', path: rung.path })}
                >
                  <Icon name="funnel" size="sm" /><ConditionWords expr={String(rung.step.params.condition ?? '')} {names} />
                  {#if verdicts?.get(rung.path)}{@const v = verdicts.get(rung.path)!}<span class="verdict inline" class:yes={v.state === 'yes'} class:no={v.state === 'no'}><Icon name={v.state === 'yes' ? 'check' : 'x'} size="sm" />{verdictWord(v)}</span>{/if}
                </button>
                {#if marks?.get(rung.path)}<span class="flag"><Icon name="triangle-alert" size="sm" />{marks.get(rung.path)}</span>{/if}
              {/if}
            </div>
            <div class="rsteps">
              {#if (rung.step.then ?? []).length === 0}
                <span class="empty">{t('app.flow.card_arm_empty')}</span>
                <InsertMenu {kinds} {summaries} {usage} {actions} list={`${rung.path}/then`} index={0} onpick={oninsert} {drag} {ondrop} />
              {:else}
                <RuleCanvasList {summaries} {usage} steps={rung.step.then ?? []} {actions} prefix={`${rung.path}/then`} {kinds} {names} {selection} {marks} {describe} {onselect} {oninsert} {onremove} {onfold} {onnudge} {onaddrung} {drag} {ondragchange} {ondrop} {segmented} {armChoice} {onpickarm} {verdicts} {dimUnvisited} />
              {/if}
            </div>
          </div>
        {/each}
        <div class="rung else" data-rung={`${last.path}/else`}>
          <div class="rhead"><span class="rpill">{t('app.flow.card_otherwise')}</span></div>
          <div class="rsteps">
            {#if (last.step.else ?? []).length === 0}
              <span class="empty">{t('app.flow.card_arm_empty')}</span>
              <InsertMenu {kinds} {summaries} {usage} {actions} list={`${last.path}/else`} index={0} onpick={oninsert} {drag} {ondrop} />
            {:else}
              <RuleCanvasList {summaries} {usage} steps={last.step.else ?? []} {actions} prefix={`${last.path}/else`} {kinds} {names} {selection} {marks} {describe} {onselect} {oninsert} {onremove} {onfold} {onnudge} {onaddrung} {drag} {ondragchange} {ondrop} {segmented} {armChoice} {onpickarm} {verdicts} {dimUnvisited} />
            {/if}
          </div>
        </div>
      </div>
      <button class="addrung" type="button" data-add-rung={path} onclick={() => onaddrung(path)}><Icon name="plus" size="sm" />{t('app.flow.add_else_if')}</button>
      {#if !endsAllPaths(step)}<span class="stub"></span>{/if}
    {:else}
      {@const seg = segmented || depthOf(`${path}/then`) >= 2}
      {@const shown = armChoice.get(path) ?? 'then'}
      {@const join = joinOf(step)}
      <span class="stub"></span>
      <div class="arms" class:seg data-branch={path}>
        {#if seg}
          <div class="seghead" role="tablist">
            {#each arms(step) as { arm, list } (arm)}
              <button type="button" role="tab" aria-selected={shown === arm} data-arm-pick={`${path}/${arm}`} onclick={() => onpickarm(path, arm)}>
                <Icon name={arm === 'then' ? 'check' : 'x'} size="sm" />{arm === 'then' ? t('app.flow.card_then') : t('app.flow.card_otherwise')} ({countSteps(list)})
              </button>
            {/each}
          </div>
        {:else}
          <!-- The fork's bar, from the one arm's centre to the other's, a corner at each end. -->
          <span class="bar"></span>
        {/if}
        {#each arms(step) as { arm, list } (arm)}
          {#if !seg || shown === arm}
            <div class="arm" data-arm={`${path}/${arm}`}>
              {#if !seg}<span class="stub"></span><span class="armlabel"><Icon name={arm === 'then' ? 'check' : 'x'} size="sm" />{arm === 'then' ? t('app.flow.card_then') : t('app.flow.card_otherwise')}</span>{/if}
              {#if list.length === 0}<span class="stub"></span>{/if}
              {#if list.length === 0}
                <span class="empty">{t('app.flow.card_arm_empty')}</span>
                <InsertMenu {kinds} {summaries} {usage} {actions} list={`${path}/${arm}`} index={0} onpick={oninsert} {drag} {ondrop} />
              {:else}
                <RuleCanvasList {summaries} {usage} steps={list} {actions} prefix={`${path}/${arm}`} {kinds} {names} {selection} {marks} {describe} {onselect} {oninsert} {onremove} {onfold} {onnudge} {onaddrung} {drag} {ondragchange} {ondrop} {segmented} {armChoice} {onpickarm} {verdicts} {dimUnvisited} />
              {/if}
              {#if continues(list)}<span class="tail"></span>{/if}
            </div>
          {/if}
        {/each}
        {#if !seg}
          <!-- The join, drawn per arm: whole, half with its corner, or none; then the stem on. -->
          <span class="join {join}"></span>
          {#if join !== 'none'}<span class="after"></span>{/if}
        {/if}
      </div>
      <button class="addrung" type="button" data-add-rung={path} onclick={() => onaddrung(path)}><Icon name="plus" size="sm" />{t('app.flow.add_else_if')}</button>
      {#if !endsAllPaths(step)}<span class="stub"></span>{/if}
    {/if}
  {/if}

  {#if step.kind === 'BRANCH' && endsAllPaths(step)}
    <!-- Every arm ends the run: the list ends here, and nothing may follow (decision 19). -->
    <span class="endcap" data-end={path}><i></i>{t('app.flow.run_ends_every_path')}</span>
  {:else if !endsAllPaths(step)}
    <InsertMenu
      {kinds}
      {summaries}
      {usage}
      {actions}
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

  /* The branch's condition on its card, in the gate's notation: the funnel, the sentences, the chips. */
  .cond { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050); font-size: var(--fs-075); }

  .cmark { display: inline-grid; place-items: center; width: var(--sp-200); height: var(--sp-200); border-radius: var(--r-xs); background: var(--label-amber-bg); color: var(--label-amber-fg); }

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

  .card.unreachable { opacity: 0.38; }

  .never { font-size: var(--fs-050); font-weight: var(--fw-medium); text-transform: uppercase; color: var(--text-subtle); padding: var(--sp-100) 0 var(--sp-050); }

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

  .stub, .tail, .after { width: var(--bw-ring); height: var(--sp-150); background: var(--border-default); border-radius: var(--r-full); flex: 0 0 auto; }

  .tail { flex: 1 1 auto; min-height: var(--sp-150); }

  /* The fork: one column per arm with a gap between them, so the arms never touch. The bar runs
     from the one arm's centre to the other's - (W - gap) / 4 in from each edge - with a corner
     turning down at each end; the join is the same bar drawn per arm; the stem after it carries
     on to the chain (decision 20). `grid-column: 1 / -1` on every line, or it sits in a column. */
  .arms {
    position: relative;
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    column-gap: var(--sp-300);
    width: min(92ch, 100%);
    --half: calc(25% - var(--sp-300) / 4);
  }

  .bar, .join, .after { grid-column: 1 / -1; justify-self: stretch; }

  .after { justify-self: center; }

  .bar, .join { position: relative; height: var(--bw-ring); margin-inline: var(--half); background: var(--border-default); border-radius: var(--r-full); }

  .bar::before, .bar::after, .join::before, .join::after { content: ''; position: absolute; width: var(--bw-ring); height: var(--sp-050); background: var(--border-default); }

  .bar::before, .bar::after { inset-block-start: 0; }

  .join::before, .join::after { inset-block-end: 0; }

  .bar::before, .join::before { inset-inline-start: 0; }

  .bar::after, .join::after { inset-inline-end: 0; }

  .join.l { margin-inline: var(--half) 50%; }

  .join.r { margin-inline: 50% var(--half); }

  .join.none { background: transparent; }

  .join.l::after, .join.r::before, .join.none::before, .join.none::after { display: none; }

  .arm { display: flex; flex-direction: column; align-items: center; min-width: 0; }

  .arm .empty { width: 100%; }

  .arm :global(.card) { width: 100%; box-sizing: border-box; }

  .armlabel {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-050) var(--sp-100);
    border-radius: var(--r-full);
    background: var(--label-violet-bg);
    color: var(--label-violet-fg);
    font-size: var(--fs-050);
    font-weight: var(--fw-medium);
    line-height: 1;
    text-transform: uppercase;
  }

  .armlabel :global(svg) { display: block; }

  .empty { padding: var(--sp-100); border: var(--bw-hairline) dashed var(--border-default); border-radius: var(--r-md); text-align: center; font-size: var(--fs-075); color: var(--text-subtle); }

  /* The ladder: rungs one under the other, each a pill and its steps indented behind a rail;
     an else-if rung's condition is a card of its own in the gate's notation. */
  .ladder { width: min(56ch, 100%); display: flex; flex-direction: column; gap: var(--sp-050); }

  .rung { display: flex; flex-direction: column; gap: var(--sp-050); padding: var(--sp-100); border: var(--bw-hairline) solid var(--label-violet-fg); border-radius: var(--r-md); background: var(--bg-surface); }

  .rung.else { border-style: dashed; }

  .rhead { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .rpill { flex: 0 0 auto; padding: var(--sp-025) var(--sp-100); border-radius: var(--r-full); background: var(--label-violet-bg); color: var(--label-violet-fg); font-size: var(--fs-050); font-weight: var(--fw-medium); line-height: 1; text-transform: uppercase; }

  .rcond { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050) var(--sp-050); padding: var(--sp-050) var(--sp-100); border: var(--bw-hairline) solid transparent; border-radius: var(--r-sm); background: var(--label-amber-bg); color: var(--text-primary); font-size: var(--fs-075); text-align: start; cursor: pointer; }

  .rcond :global(svg) { color: var(--label-amber-fg); }

  .rcond:hover { border-color: var(--label-amber-fg); }

  .rcond.selected { outline: var(--bw-ring) solid var(--accent-primary); outline-offset: var(--sp-025); }

  .rcond.flagged { border-color: var(--status-warning-border); }

  .rcond:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .rsteps { display: flex; flex-direction: column; align-items: stretch; min-width: 0; margin-inline-start: var(--sp-200); padding-inline-start: var(--sp-150); border-inline-start: var(--bw-ring) solid var(--border-subtle); }

  .rsteps :global(.card) { width: 100%; box-sizing: border-box; }

  .verdict.inline { position: static; }

  /* + Else if, under every branch. */
  .addrung { display: inline-flex; align-items: center; gap: var(--sp-050); margin-block: var(--sp-050); padding: var(--sp-025) var(--sp-100); border: var(--bw-hairline) dashed var(--border-default); border-radius: var(--r-full); background: var(--bg-surface); color: var(--text-secondary); font-size: var(--fs-075); cursor: pointer; }

  .addrung:hover { border-color: var(--label-violet-fg); color: var(--label-violet-fg); }

  .addrung:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  /* The end mark: a stem and a square, where every path of the list has ended. */
  .endcap { display: inline-flex; flex-direction: column; align-items: center; gap: var(--sp-050); font-size: var(--fs-050); font-weight: var(--fw-medium); text-transform: uppercase; color: var(--text-subtle); }

  .endcap::before { content: ''; display: block; width: var(--bw-ring); height: var(--sp-150); background: var(--border-default); }

  .endcap i { display: block; width: var(--sp-150); height: var(--sp-150); border-radius: var(--r-xs); background: var(--border-strong); }

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
