<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workflow an entry runs, drawn read only (E-23, work-surface concept §6.9): a lane per
  // category, the statuses as boxes, each step an arrow from the status it leaves to the one it
  // leads to, the entry's status outlined; the arrows the reader can take now in the accent, the
  // ones blocked for them dashed. Below, a card per step says who may take it, what it asks and
  // whether the reader may. The server decides all of that (`GET /items/{id}/steps`, rule 2); who
  // exactly is named for the steps from the entry's status only, as far as the answer names it.
  //
  // The arrows are measured from the boxes after layout, so no length is written here (rule 15).
  import { tick } from 'svelte';
  import { StateLozenge } from '@hubtask/design-system/components';
  import type { EntrySteps } from '@hubtask/sync-engine';
  import { statusLabel } from '../data/workflows.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  type Picture = NonNullable<EntrySteps['picture']>;
  type PictureStep = Picture['steps'][number];
  type Kind = 'now' | 'blocked' | 'other';
  type Anchor = 'middle' | 'start' | 'end';

  interface Props {
    picture: Picture;
    /** The entry's status. */
    current: string;
    /** The steps the reader may take from the entry's status, by key, with what each asks. */
    allowed: ReadonlyMap<string, string | undefined>;
    /** The steps from the entry's status the reader may not take, by key, with why. */
    blocked: ReadonlyMap<string, string>;
  }

  const { picture, current, allowed, blocked }: Props = $props();

  const instanceId = $props.id();
  const markerId = `workflow-picture-${instanceId}`;
  const KINDS: readonly Kind[] = ['now', 'blocked', 'other'];
  const ORDER = ['BACKLOG', 'TODO', 'IN_PROGRESS', 'DONE'] as const;
  const lanes = $derived(
    ORDER.map((category) => ({ category, statuses: picture.statuses.filter((each) => each.category === category) }))
      .filter((lane) => lane.statuses.length > 0),
  );
  const nameOf = (key: string) => {
    const status = picture.statuses.find((each) => each.key === key);
    return status ? statusLabel(status, t) : key;
  };
  const stepName = (step: PictureStep) => step.name ?? nameOf(step.to);
  const leavesHere = (step: PictureStep) => (step.from.length === 0 ? step.to !== current : step.from.includes(current));
  const kindOf = (step: PictureStep): Kind =>
    leavesHere(step) && allowed.has(step.key) ? 'now' : leavesHere(step) && blocked.has(step.key) ? 'blocked' : 'other';
  // A step from any status is an arrow from the entry's status; only on the entry's own status,
  // where no arrow leaves, is it written on the box.
  const anyInto = (key: string) => (key === current ? picture.steps.filter((step) => step.from.length === 0 && step.to === key) : []);

  let canvas = $state<HTMLDivElement | undefined>(undefined);
  const boxes = new Map<string, HTMLElement>();
  let extent = $state({ width: 0, height: 0 });
  let arrows = $state<readonly { key: string; d: string; kind: Kind; label: string; x: number; y: number; anchor: Anchor }[]>([]);
  const labels = new Map<string, SVGTextElement>();
  /** How far each name is moved down so that no two names overlap, by arrow. */
  let shifts = $state<ReadonlyMap<string, number>>(new Map());

  function box(node: HTMLElement, key: string) {
    boxes.set(key, node);
    return { destroy: () => boxes.delete(key) };
  }
  function label(node: SVGTextElement, key: string) {
    labels.set(key, node);
    return { destroy: () => labels.delete(key) };
  }

  /** Moves a name down past every name above it that it would overlap, top to bottom. */
  function separate() {
    const placed: { x: number; y: number; width: number; height: number }[] = [];
    const out = new Map<string, number>();
    const measured = arrows
      .map((arrow) => ({ key: arrow.key, rect: labels.get(arrow.key)?.getBBox() }))
      .filter((each): each is { key: string; rect: DOMRect } => each.rect !== undefined && each.rect.width > 0)
      .sort((a, b) => a.rect.y - b.rect.y || a.rect.x - b.rect.x);
    for (const { key, rect } of measured) {
      const at = { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
      for (let moved = true; moved; ) {
        moved = false;
        for (const other of placed) {
          const apart = at.x >= other.x + other.width || other.x >= at.x + at.width || at.y >= other.y + other.height || other.y >= at.y + at.height;
          if (apart) continue;
          at.y = other.y + other.height + at.height / 8;
          moved = true;
        }
      }
      placed.push(at);
      if (at.y !== rect.y) out.set(key, at.y - rect.y);
    }
    shifts = out;
  }

  /** One arrow per step and status it leaves, along the axis the boxes lie apart on; two opposite
   *  steps between the same boxes side by side; a long way back around the boxes between. */
  function measure() {
    if (!canvas) return;
    const origin = canvas.getBoundingClientRect();
    extent = { width: origin.width, height: origin.height };
    const rect = (key: string) => {
      const r = boxes.get(key)?.getBoundingClientRect();
      if (!r) return undefined;
      const left = r.left - origin.left;
      const top = r.top - origin.top;
      return { xMin: left, yMin: top, xMax: left + r.width, yMax: top + r.height, width: r.width, height: r.height, cx: left + r.width / 2, cy: top + r.height / 2 };
    };
    const lowest = Math.max(0, ...[...boxes.keys()].map((key) => rect(key)?.yMax ?? 0));
    /** Whether another box stands between two boxes of one column, so a straight arrow would cross it. */
    const crosses = (from: string, to: string) => {
      const a = rect(from);
      const b = rect(to);
      if (!a || !b) return false;
      const [top, bottom] = a.cy < b.cy ? [a.cy, b.cy] : [b.cy, a.cy];
      return [...boxes.keys()].some((key) => {
        if (key === from || key === to) return false;
        const c = rect(key);
        return Boolean(c && c.cy > top && c.cy < bottom && c.xMin < Math.max(a.xMax, b.xMax) && c.xMax > Math.min(a.xMin, b.xMin));
      });
    };
    const pairs = new Set(picture.steps.flatMap((step) => step.from.map((from) => `${from}>${step.to}`)));
    const out: { key: string; d: string; kind: Kind; label: string; x: number; y: number; anchor: Anchor }[] = [];
    for (const step of picture.steps) {
      const target = rect(step.to);
      if (!target) continue;
      const froms = step.from.length > 0 ? step.from : step.to === current ? [] : [current];
      for (const from of froms) {
        const source = rect(from);
        if (!source || from === step.to) continue;
        const dx = target.cx - source.cx;
        const dy = target.cy - source.cy;
        const side = pairs.has(`${step.to}>${from}`) ? (from < step.to ? -1 : 1) : 0;
        let d: string;
        let x: number;
        let y: number;
        let anchor: Anchor = 'middle';
        let isBelow = false;
        if (Math.abs(dx) >= Math.abs(dy) && dx < 0 && -dx > source.width * 1.5) {
          // A long way back: out into the gap beside its lane and under every box, so it crosses
          // none of them.
          const low = lowest + source.height / 2;
          const aside = source.xMin - source.height / 3;
          d = `M ${source.xMin} ${source.cy} L ${aside} ${source.cy} L ${aside} ${low} L ${target.cx} ${low} L ${target.cx} ${target.yMax}`;
          x = (aside + target.cx) / 2;
          y = low;
          isBelow = true;
        } else if (Math.abs(dx) >= Math.abs(dy)) {
          const off = (side * source.height) / 4;
          const x1 = dx > 0 ? source.xMax : source.xMin;
          const x2 = dx > 0 ? target.xMin : target.xMax;
          const y1 = source.cy + off;
          const y2 = target.cy + off;
          const bend = (x2 - x1) / 2;
          d = `M ${x1} ${y1} C ${x1 + bend} ${y1}, ${x2 - bend} ${y2}, ${x2} ${y2}`;
          x = (x1 + x2) / 2;
          y = (y1 + y2) / 2;
        } else if (dy < 0 && crosses(from, step.to)) {
          // A long way up: beside the boxes between.
          const edge = Math.max(0, Math.min(source.xMin, target.xMin) - source.width / 4);
          d = `M ${source.xMin} ${source.cy} C ${edge} ${source.cy}, ${edge} ${target.cy}, ${target.xMin} ${target.cy}`;
          x = edge;
          y = (source.cy + target.cy) / 2;
          anchor = 'start';
        } else {
          const off = (side * source.width) / 6;
          const y1 = dy > 0 ? source.yMax : source.yMin;
          const y2 = dy > 0 ? target.yMin : target.yMax;
          const x1 = source.cx + off;
          const x2 = target.cx + off;
          const bend = (y2 - y1) / 2;
          d = `M ${x1} ${y1} C ${x1} ${y1 + bend}, ${x2} ${y2 - bend}, ${x2} ${y2}`;
          x = (x1 + x2) / 2;
          y = (y1 + y2) / 2;
          anchor = off < 0 ? 'end' : 'start';
        }
        // The name beside its line, a quarter of a box's height off it.
        const gap = source.height / 4;
        if (anchor === 'middle') y += isBelow ? gap * 2 : -gap;
        else x += anchor === 'start' ? gap : -gap;
        // A step from any status names itself under its target, clear of the boxes it passes.
        if (step.from.length === 0) {
          x = target.xMin;
          y = target.yMax + gap * 2;
          anchor = 'start';
        }
        const name = step.from.length === 0 ? t('app.workflows.picture_any_into', { name: stepName(step) }) : stepName(step);
        out.push({ key: `${step.key}:${from}`, d, kind: kindOf(step), label: name, x, y, anchor });
      }
    }
    arrows = out;
    shifts = new Map();
    void tick().then(separate);
  }

  $effect(() => {
    void picture;
    void current;
    if (!canvas) return;
    const frame = requestAnimationFrame(measure);
    const observer = new ResizeObserver(() => measure());
    observer.observe(canvas);
    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
    };
  });

  function whoLine(step: PictureStep): string {
    const reason = leavesHere(step) ? blocked.get(step.key) : undefined;
    if (reason !== undefined) return reason;
    return t('app.workflows.card_who', { who: step.limited ? t('app.workflows.picture_limited') : t('app.workflows.card_anyone') });
  }
  function asksLine(step: PictureStep): string | undefined {
    if (leavesHere(step) && allowed.has(step.key)) return allowed.get(step.key);
    return step.approval ? t('app.workflows.needs_approval') : undefined;
  }
</script>

<div class="picture" data-workflow-picture>
  <div class="canvas" bind:this={canvas}>
    <svg class="arrows" width={extent.width} height={extent.height} aria-hidden="true">
      <defs>
        {#each KINDS as kind (kind)}
          <marker id={`${markerId}-${kind}`} viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M 0 0 L 10 5 L 0 10 z" class="head" data-kind={kind} />
          </marker>
        {/each}
      </defs>
      {#each arrows as arrow (arrow.key)}
        <path d={arrow.d} class="arrow" data-kind={arrow.kind} data-arrow={arrow.key} marker-end={`url(#${markerId}-${arrow.kind})`} />
      {/each}
      {#each arrows as arrow (arrow.key)}
        <text
          class="label"
          data-kind={arrow.kind}
          use:label={arrow.key}
          data-label={arrow.key}
          x={arrow.x}
          y={arrow.y + (shifts.get(arrow.key) ?? 0)}
          text-anchor={arrow.anchor}
          dominant-baseline={arrow.anchor === 'middle' ? 'auto' : 'middle'}
        >{arrow.label}</text>
      {/each}
    </svg>
    {#each lanes as lane (lane.category)}
      <section class="lane" aria-label={t(`workflows.category.${lane.category.toLowerCase()}`)}>
        <h3 class="heading">{t(`workflows.category.${lane.category.toLowerCase()}`)}</h3>
        <ol class="boxes">
          {#each lane.statuses as status (status.key)}
            <li class="box" use:box={status.key} data-category={status.category} data-current={status.key === current ? '' : undefined} data-picture-status={status.key}>
              <span class="name">{statusLabel(status, t)}</span>
              {#if status.key === current}<span class="here">{t('app.workflows.picture_here')}</span>{/if}
              {#each anyInto(status.key) as step (step.key)}
                <span class="any">{t('app.workflows.picture_any_into', { name: stepName(step) })}</span>
              {/each}
            </li>
          {/each}
        </ol>
      </section>
    {/each}
  </div>

  <ul class="legend" data-picture-legend>
    <li><span class="sample" data-sample="current"></span>{t('app.workflows.legend_current')}</li>
    <li><span class="sample" data-sample="now"></span>{t('app.workflows.legend_now')}</li>
    <li><span class="sample" data-sample="other"></span>{t('app.workflows.legend_other')}</li>
    <li><span class="sample" data-sample="blocked"></span>{t('app.workflows.legend_blocked')}</li>
  </ul>

  <h3 class="heading steps">{t('app.workflows.steps_heading')}</h3>
  <ul class="cards">
    {#each picture.steps as step (step.key)}
      {@const kind = kindOf(step)}
      {@const asks = asksLine(step)}
      <li class="card" data-step-card={step.key} data-kind={kind}>
        <span class="card-head">
          <span class="card-name">{stepName(step)}</span>
          {#if kind === 'now'}<StateLozenge label={t('app.workflows.card_can')} tone="success" icon="check" size="sm" />{/if}
          {#if kind === 'blocked'}<StateLozenge label={t('app.workflows.card_blocked')} tone="neutral" icon="lock" size="sm" />{/if}
        </span>
        <span class="card-line">{step.from.length === 0 ? t('app.workflows.picture_any') : step.from.map(nameOf).join(', ')} → {nameOf(step.to)}</span>
        <span class="card-line">{whoLine(step)}</span>
        {#if asks}<span class="card-line">{asks}</span>{/if}
      </li>
    {/each}
  </ul>
</div>

<style>
  .picture { display: flex; flex-direction: column; gap: var(--sp-200); min-inline-size: 0; }

  .canvas {
    position: relative;
    display: grid;
    grid-auto-flow: column;
    grid-auto-columns: minmax(0, 1fr);
    padding: var(--sp-150) var(--sp-150) var(--sp-600);
    border-radius: var(--r-lg);
    background: var(--bg-surface-sunken);
  }
  .arrows { position: absolute; inset-block-start: 0; inset-inline-start: 0; pointer-events: none; overflow: visible; }
  .arrow { fill: none; stroke: var(--border-strong); stroke-width: var(--bw-thick); }
  .arrow[data-kind='now'] { stroke: var(--accent-primary); }
  .arrow[data-kind='blocked'] { stroke-dasharray: var(--sp-050) var(--sp-050); }
  .head { fill: var(--border-strong); }
  .head[data-kind='now'] { fill: var(--accent-primary); }
  .label {
    font-size: var(--fs-065);
    fill: var(--text-secondary);
    paint-order: stroke;
    stroke: var(--bg-surface-sunken);
    stroke-width: var(--sp-050);
    stroke-linejoin: round;
  }
  .label[data-kind='now'] { fill: var(--text-brand); font-weight: var(--fw-semibold); }

  .lane { display: flex; flex-direction: column; gap: var(--sp-200); min-inline-size: 0; padding-inline: var(--sp-300); }
  .lane + .lane { border-inline-start: var(--bw-hairline) solid var(--border-subtle); }
  .heading { margin: 0; font-size: var(--fs-065); font-weight: var(--fw-semibold); color: var(--text-secondary); text-transform: uppercase; text-align: center; }
  .heading.steps { text-align: start; }
  .boxes { display: flex; flex-direction: column; gap: var(--sp-600); margin: 0; padding: 0; list-style: none; }
  .box {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--sp-025);
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-strong);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    text-align: center;
  }
  .box[data-category='DONE'] { background: var(--status-success-surface); }
  .box[data-current] { border: var(--bw-thick) solid var(--accent-primary); }
  .name { font-size: var(--fs-075); font-weight: var(--fw-semibold); overflow-wrap: anywhere; }
  .here { font-size: var(--fs-065); color: var(--text-brand); }
  .any { font-size: var(--fs-065); color: var(--text-secondary); }

  .legend { display: flex; flex-wrap: wrap; gap: var(--sp-100) var(--sp-300); margin: 0; padding: 0; list-style: none; font-size: var(--fs-065); color: var(--text-secondary); }
  .legend li { display: inline-flex; align-items: center; gap: var(--sp-100); }
  .sample { display: inline-block; inline-size: var(--sp-300); border-block-start: var(--bw-thick) solid var(--border-strong); }
  .sample[data-sample='now'] { border-color: var(--accent-primary); }
  .sample[data-sample='blocked'] { border-block-start-style: dashed; }
  .sample[data-sample='current'] { inline-size: var(--sp-250); block-size: var(--sp-150); border: var(--bw-thick) solid var(--accent-primary); border-radius: var(--r-full); }

  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--sp-1600) + var(--sp-400)), 1fr));
    gap: var(--sp-100);
    margin: 0;
    padding: 0;
    list-style: none;
  }
  .card {
    display: flex;
    flex-direction: column;
    gap: var(--sp-025);
    min-inline-size: 0;
    padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    font-size: var(--fs-075);
    color: var(--text-secondary);
  }
  .card[data-kind='now'] { border-color: var(--accent-primary); background: var(--accent-primary-subtle); }
  .card[data-kind='blocked'] { color: var(--text-subtle); }
  .card-head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-050); }
  .card-name { font-weight: var(--fw-semibold); color: var(--text-primary); overflow-wrap: anywhere; }
  .card[data-kind='blocked'] .card-name { color: var(--text-secondary); }
  .card-line { overflow-wrap: anywhere; }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .canvas { grid-auto-flow: row; grid-auto-columns: auto; gap: var(--sp-400); padding: var(--sp-150) var(--sp-150) var(--sp-150) var(--sp-500); }
    .lane { padding-inline: 0; }
    .lane + .lane { border-inline-start: none; }
    .heading { text-align: start; }
    .boxes { flex-direction: row; flex-wrap: wrap; gap: var(--sp-200) var(--sp-500); }
    .box { flex: 1 1 0; min-inline-size: 0; }
    .label { display: none; }
  }
</style>
