<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The picture on an entry.
  //
  // **The manifest decides whether there is one at all.** A cover belongs to a type whose profile
  // carries `COVER` — a task by default, and neither a work package nor an activity — and this
  // component reads that rather than naming the types. A type without it is a gate with the
  // server's own reason, never a missing control.
  //
  // **Three steps, and the middle one leaves this origin.** Staging answers where the bytes go,
  // the bytes go there directly (arc42 §8.4), and the confirmation is what judges them and makes
  // the object usable. Only then is it a cover: setting one from a PENDING object is refused, and
  // rightly — the type has been claimed and not yet read.
  //
  // **The image is drawn from the download URL the server answers for a READY object.** That is
  // the one path the sniffed inline allowlist has already judged (C-05). Nothing here draws from a
  // blob the browser was handed, which would be this client rendering bytes nobody looked at.
  //
  // A cancelled upload leaves a staging behind and this says so. The reconciliation job takes it;
  // deleting it here would be guessing at whether the bytes ever arrived.
  //
  // **Both kinds, because the contract has two and only one was reachable.** `CoverInput` takes a
  // colour token or a media identifier, `WorkItemCard` has drawn both since F9, and no client
  // could ever set the colour: this panel offered an upload and nothing else. ADR-0063 decision 9
  // makes the row the place a cover is decided, and a place that offers one of two kinds is not
  // that. The ten are the design system's, the same ten a label chooses from - a colour that is
  // measured against both themes rather than picked off a wheel.

  import { labelTokens } from '@hubtask/design-system';
  import { Button, CapabilityGate, Icon, Stack, UploadField } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { supports } from '../data/capability.svelte.ts';
  import { media } from '../data/media.svelte.ts';
  import { acceptFor, colourCover, coverImageIdOf, imageCover, isWithinUploadLimit, uploadLimitOf } from '../data/media.ts';
  import { formatBytes } from '../i18n/bytes.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { item }: { item: WorkItem } = $props();

  const capability = $derived(supports(item.type, 'COVER'));
  const limit = $derived(uploadLimitOf(manifest.value));

  const imageId = $derived(coverImageIdOf(item.cover));
  const isColour = $derived(item.cover?.kind === 'COLOR' && !!item.cover.color_token);
  /** The token on the entry, where it is one this build knows how to paint. */
  const chosenToken = $derived(
    item.cover?.kind === 'COLOR' && item.cover.color_token
      ? (labelTokens as readonly string[]).find((token) => token === item.cover?.color_token)
      : undefined,
  );

  // `Date.now()` rather than a clock this component holds: what it answers only decides whether the
  // target the cache holds has expired, and the cache asks again when it has.
  const url = $derived(imageId ? media.coverUrl(imageId, Date.now()) : undefined);

  let chosen = $state<File | undefined>(undefined);
  let sent = $state(0);
  let controller = $state<AbortController | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);

  const isUploading = $derived(controller !== undefined);
  const overLimit = $derived(chosen !== undefined && !isWithinUploadLimit(chosen.size, limit));
  const percent = $derived(chosen && chosen.size > 0 ? Math.floor((sent / chosen.size) * 100) : 0);

  const sizeLabel = $derived(
    chosen === undefined
      ? undefined
      : limit === undefined
        ? t('app.media.size', { size: formatBytes(chosen.size, messages.locale) })
        : t('app.media.size_of_limit', {
            size: formatBytes(chosen.size, messages.locale),
            limit: formatBytes(limit, messages.locale),
          }),
  );

  async function choose(file: File) {
    chosen = file;
    failure = undefined;
    sent = 0;
    // Refused before a byte leaves, from the installation's own ceiling. The alternative is a
    // person watching a large file upload and being told at confirmation that it was never
    // acceptable.
    if (!isWithinUploadLimit(file.size, limit)) return;

    const abort = new AbortController();
    controller = abort;
    try {
      const object = await media.upload(file, 'COVER', {
        idempotencyKey: crypto.randomUUID(),
        signal: abort.signal,
        onProgress: (moved) => (sent = moved),
      });
      await media.setCover(item.id, imageCover(object.id), item.version);
      announcer.say(t('app.cover.set_announced'));
      chosen = undefined;
    } catch (error) {
      failure = abort.signal.aborted
        ? t('app.media.cancelled')
        : renderProblem(error as never, messages).message;
    } finally {
      controller = undefined;
    }
  }

  async function chooseColour(token: string) {
    failure = undefined;
    try {
      await media.setCover(item.id, colourCover(token), item.version);
      announcer.say(t('app.cover.set_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }

  async function remove() {
    failure = undefined;
    try {
      await media.clearCover(item.id, item.version);
      announcer.say(t('app.cover.cleared_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

<CapabilityGate
  status={capability.status}
  reason={capability.status === 'refused' ? t(capability.code, capability.params) : undefined}
  pendingLabel={t('app.media.deciding')}
>
  <Stack gap="150">
    <!-- What the row is for, in one sentence: where a cover is drawn. Unset, that is the whole of
         what this panel has to say before it offers the two kinds (ADR-0063 decision 9). -->
    <p class="quiet">{t('app.media.cover_where')}</p>

    {#if imageId}
      {#if url}
        <img class="cover" src={url} alt={t('app.media.cover_alt', { title: item.title })} />
      {:else}
        <p class="quiet">{t('app.media.cover_drawing')}</p>
      {/if}
    {/if}

    <!-- The ten, as a group of toggles: one press is one write, because a cover is one field and a
         save button over a single choice is a step for nothing. Rule 3 keeps the chosen one
         readable without colour - the tick says which, the ring says where the focus is. -->
    <fieldset class="colours">
      <legend>{t('app.media.cover_colour_legend')}</legend>
      {#each labelTokens as token (token)}
        <button
          type="button"
          class="swatch"
          data-token={token}
          aria-pressed={chosenToken === token}
          aria-label={t(`app.colour.${token}`)}
          onclick={() => void chooseColour(token)}
        >
          {#if chosenToken === token}<Icon name="check" size="sm" />{/if}
        </button>
      {/each}
    </fieldset>

    <UploadField
      label={t('app.media.cover_picture')}
      hint={t('app.media.cover_hint')}
      chooseLabel={imageId ? t('app.media.cover_replace') : t('app.media.cover_choose')}
      dropLabel={t('app.media.drop')}
      cancelLabel={t('app.media.cancel')}
      accept={acceptFor('COVER')}
      file={chosen ?? null}
      {sizeLabel}
      overLimitLabel={overLimit ? t('app.media.over_limit') : undefined}
      progress={isUploading ? percent : undefined}
      progressLabel={isUploading ? t('app.media.uploading', { percent: String(percent) }) : undefined}
      onChoose={(file) => void choose(file)}
      onCancel={() => controller?.abort()}
    />

    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

    {#if imageId || isColour}
      <div>
        <Button size="sm" tone="secondary" onclick={() => void remove()}>
          {t('app.media.cover_remove')}
        </Button>
      </div>
    {/if}
  </Stack>
</CapabilityGate>

<style>
  .cover {
    display: block;
    width: 100%;
    max-width: var(--sp-1600);
    height: auto;
    border-radius: var(--r-md);
  }

  .quiet { margin: 0; color: var(--text-secondary); }

  .colours {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-050);
    margin: 0;
    padding: 0;
    border: 0;
  }

  .colours legend {
    padding: 0;
    margin-block-end: var(--sp-050);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  .swatch {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    inline-size: var(--density-control-sm-min);
    block-size: var(--density-control-sm-min);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-sm);
    padding: 0;
    color: var(--text-primary);
    cursor: pointer;
  }

  .swatch:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .swatch[aria-pressed='true'] { outline: var(--bw-ring) solid var(--accent-primary); }

  .swatch[data-token='slate'] { background: var(--label-slate-bg); color: var(--label-slate-fg); }
  .swatch[data-token='blue'] { background: var(--label-blue-bg); color: var(--label-blue-fg); }
  .swatch[data-token='teal'] { background: var(--label-teal-bg); color: var(--label-teal-fg); }
  .swatch[data-token='green'] { background: var(--label-green-bg); color: var(--label-green-fg); }
  .swatch[data-token='lime'] { background: var(--label-lime-bg); color: var(--label-lime-fg); }
  .swatch[data-token='amber'] { background: var(--label-amber-bg); color: var(--label-amber-fg); }
  .swatch[data-token='orange'] { background: var(--label-orange-bg); color: var(--label-orange-fg); }
  .swatch[data-token='red'] { background: var(--label-red-bg); color: var(--label-red-fg); }
  .swatch[data-token='magenta'] { background: var(--label-magenta-bg); color: var(--label-magenta-fg); }
  .swatch[data-token='violet'] { background: var(--label-violet-bg); color: var(--label-violet-fg); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
