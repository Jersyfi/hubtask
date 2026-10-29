<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The manifest, made visible. This is the view that proves `/meta/capabilities` is read and used
  // rather than fetched and ignored - and it is also the deep link the reload test uses, because
  // it is a real second route rather than a fragment.

  import { Badge, Button, Icon, Inline, PageHeader, Spinner, Stack } from '@hubtask/design-system/components';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { appUpdate } from '../lib/frame/appupdate.svelte.ts';
  import { platform } from '../lib/platform/index.ts';
  import { releasePage } from '../lib/platform/update.ts';

  const state = $derived(manifest.state);
  const problem = $derived(state.status === 'failed' ? renderProblem(state.error, messages) : undefined);
  // The installed apps call the page what the account menu does, since it is about the app too.
  const title = $derived(appUpdate.current === undefined ? t('app.installation.title') : t('app.nav.about'));
  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(title));
  const found = $derived(appUpdate.found);
  const server = $derived.by(() => {
    try {
      return new URL(platform.webOrigin).host;
    } catch {
      return platform.webOrigin;
    }
  });
</script>

<Stack gap="300">
  <PageHeader {title} isTitleInBar={viewport.isCompact} />

  {#if appUpdate.current !== undefined}
    <!-- The installed app's own version and what it knows of a newer one (UC-SYN-07 checks 5 and
         6): the offer stays here after *Later* hid its banner. -->
    <section class="panel" data-this-app>
      <span class="label">{t('app.update.this_app')}</span>
      <p class="fixed">{appUpdate.current}</p>
      {#if found && found.state !== 'refused'}
        <div>
          <Badge tone="info">
            {found.state === 'ready'
              ? t('app.update.badge_ready', { version: found.version })
              : t('app.update.badge_available', { version: found.version })}
          </Badge>
        </div>
      {/if}
      <p class="quiet">
        {appUpdate.offersOn === 'today' ? t('app.update.signed_android') : t('app.update.signed_desktop')}
      </p>
      {#if found?.state === 'ready'}
        <div class="full">
          <Button
            tone="secondary"
            isFull
            isBusy={appUpdate.isInstalling}
            busyLabel={t('app.update.installing', { version: found.version })}
            onclick={() => void appUpdate.install()}
          >
            {t('app.update.install')}
          </Button>
        </div>
      {:else if found?.state === 'available'}
        <div class="full">
          <Button tone="secondary" isFull icon="download" onclick={() => appUpdate.open(releasePage(found.version))}>
            {t('app.update.download', { version: found.version })}
          </Button>
        </div>
      {/if}
    </section>
    {#if state.status === 'ready'}
      <section class="panel" data-server>
        <span class="label">{t('app.update.server')}</span>
        <p class="fixed server">{t('app.update.server_value', { host: server, version: state.data.product_version ?? '—' })}</p>
      </section>
    {/if}
  {/if}

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

  <!-- The three pages about the software, and the reason they are on the page that already
       carries the versions: what somebody quotes when they report a problem and where they report
       a barrier are the same errand, and this is the route the account group offers for it
       ("About Hubtask"), not a footer on every screen: one link does
       not earn a landmark on a board (design-system.md §10).

       Outside the manifest's three states on purpose. The statement has to be reachable when the
       server is not - a client that hid the way to the accessibility statement because
       `/meta/capabilities` timed out would hide it exactly from the reader who is having the
       worst time of it.

       They say `hubtask.eu` in the words because they leave the application, and they are about
       Hubtask rather than about this installation - the note says so, because on somebody's own
       server the operator, not the project, is who answers for the service. -->
  <section class="about">
    <h2>{t('app.about.title')}</h2>
    <p class="note">{t('app.about.note')}</p>
    <ul>
      <li>
        <a href="https://hubtask.eu/accessibility/" target="_blank" rel="noopener">
          <Icon name="external-link" size="sm" />{t('app.about.accessibility')}
        </a>
      </li>
      <li>
        <a href="https://hubtask.eu/licence/" target="_blank" rel="noopener">
          <Icon name="external-link" size="sm" />{t('app.about.licence')}
        </a>
      </li>
      <li>
        <a href="https://github.com/Jersyfi/hubtask" target="_blank" rel="noopener">
          <Icon name="external-link" size="sm" />{t('app.about.source')}
        </a>
      </li>
    </ul>
  </section>
</Stack>

<style>
  /* The installed app's panels, as the approved preview draws them. */
  .panel {
    display: flex;
    flex-direction: column;
    align-items: start;
    gap: var(--sp-100);
    max-width: 72ch;
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }

  .panel p { margin: 0; }
  .label { font-size: var(--fs-075); font-weight: var(--fw-medium); color: var(--text-primary); }
  .fixed { font-family: var(--font-mono); font-size: var(--fs-200); font-weight: var(--fw-regular); color: var(--text-primary); overflow-wrap: anywhere; }
  .full { align-self: stretch; }
  .fixed.server { font-size: var(--fs-100); }
  .quiet { color: var(--text-subtle); font-size: var(--fs-075); }

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

  .about h2 {
    margin: 0 0 var(--sp-050);
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
  }

  .about .note {
    margin: 0 0 var(--sp-150);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  .about ul {
    display: flex;
    flex-direction: column;
    align-items: start;
    gap: var(--sp-050);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .about a {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-050) 0;
    color: var(--text-brand);
    font-size: var(--fs-100);
    text-decoration: none;
  }

  .about a:hover { text-decoration: underline; }

  /* Rule 5's ring, on a link that sits at the left edge of the column. */
  .about a:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }
</style>
