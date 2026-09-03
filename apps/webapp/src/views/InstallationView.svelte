<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The manifest, made visible. This is the view that proves `/meta/capabilities` is read and used
  // rather than fetched and ignored - and it is also the deep link the reload test uses, because
  // it is a real second route rather than a fragment.

  import { Badge, Button, Inline, Spinner, Stack } from '@hubtask/design-system/components';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const state = $derived(manifest.state);
  const problem = $derived(state.status === 'failed' ? renderProblem(state.error, messages) : undefined);
</script>

<Stack gap="300">
  <h1>{t('app.installation.title')}</h1>

  {#if state.status === 'loading' || state.status === 'idle'}
    <Inline gap="150" align="center">
      <Spinner label={t('app.installation.reading')} />
      <span>{t('app.installation.reading')}</span>
    </Inline>
  {:else if problem}
    <Stack gap="150">
      <p>{t('app.installation.unread')}</p>
      <p class="detail">{problem.message}</p>
      {#if problem.reference}
        <p class="reference">{t('app.error_reference', { request_id: problem.reference })}</p>
      {/if}
      <Inline gap="150">
        <Button tone="secondary" icon="repeat" onclick={() => manifest.refresh()}>{t('app.retry')}</Button>
      </Inline>
    </Stack>
  {:else if state.status === 'ready'}
    <dl>
      <dt>{t('app.installation.product_version')}</dt>
      <dd>{state.data.product_version ?? '—'}</dd>
      <dt>{t('app.installation.api_version')}</dt>
      <dd>{state.data.api_version ?? '—'}</dd>
      <dt>{t('app.installation.tenancy')}</dt>
      <dd>{state.data.tenancy_mode ?? '—'}</dd>
      <dt>{t('app.installation.locales')}</dt>
      <dd>
        <Inline gap="100">
          {#each manifest.supportedLocales as locale (locale.locale)}
            <!-- The direction is the installation's answer, not a list compiled into the client:
                 this is the same value that turns the document round. -->
            <Badge tone={locale.locale === messages.locale ? 'info' : 'neutral'}>
              {locale.locale} · {locale.direction}
            </Badge>
          {:else}
            <span>—</span>
          {/each}
        </Inline>
      </dd>
    </dl>
  {/if}
</Stack>

<style>
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  dl {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: var(--sp-100) var(--sp-300);
    margin: 0;
    font-size: var(--fs-100);
  }

  dt { color: var(--text-subtle); }
  dd { margin: 0; }

  .detail { color: var(--text-secondary); }
  .reference { color: var(--text-subtle); font-family: var(--font-mono); font-size: var(--fs-075); }
</style>
