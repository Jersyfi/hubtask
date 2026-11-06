<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A newer release of the installed app, in the frame's banner zone (UC-SYN-07 checks 5 and 6,
  // the approved preview): ready to install on the desktop, a download on Android and for the
  // `.deb`, or refused because a signature did not hold. Nothing in it suggests ignoring the
  // signature; a refusal names what to do instead.

  import { Banner, Button } from '@hubtask/design-system/components';

  import { RELEASES, releasePage } from '../platform/update.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { appUpdate } from './appupdate.svelte.ts';

  interface Props {
    /** Whether the page is *Today*, where the Android app's notice stands. */
    isToday: boolean;
  }

  const { isToday }: Props = $props();

  const found = $derived(appUpdate.banner);
  const isHere = $derived(found !== undefined && (appUpdate.offersOn === 'every-page' || isToday));
  const current = $derived(appUpdate.current ?? '');
</script>

{#if found && isHere}
  <div class="update" data-app-update={found.state}>
    {#if found.state === 'ready'}
      <Banner tone="info" title={t('app.update.ready_title', { version: found.version })}>
        {t('app.update.ready_text', { current })}
        {#snippet action()}
          <Button
            size="sm"
            tone="primary"
            isBusy={appUpdate.isInstalling}
            busyLabel={t('app.update.installing', { version: found.version })}
            onclick={() => void appUpdate.install()}
          >
            {t('app.update.install')}
          </Button>
          <Button size="sm" tone="subtle" onclick={() => appUpdate.later()}>{t('app.update.later')}</Button>
          <a
            class="link"
            href={releasePage(found.version)}
            onclick={(event) => {
              event.preventDefault();
              appUpdate.open(releasePage(found.version));
            }}
          >
            {t('app.update.whats_new', { version: found.version })}
          </a>
        {/snippet}
      </Banner>
    {:else if found.state === 'available'}
      <Banner
        tone="info"
        title={appUpdate.offersOn === 'today'
          ? t('app.update.notice_title')
          : t('app.update.available_title', { version: found.version })}
      >
        {appUpdate.offersOn === 'today'
          ? t('app.update.notice_text', { version: found.version, current })
          : t('app.update.available_text', { current })}
        {#snippet action()}
          <Button size="sm" tone="primary" icon="download" onclick={() => appUpdate.open(releasePage(found.version))}>
            {t('app.update.download', { version: found.version })}
          </Button>
          <Button size="sm" tone="subtle" onclick={() => appUpdate.later()}>{t('app.update.later')}</Button>
        {/snippet}
      </Banner>
    {:else}
      <Banner tone="danger" title={t('app.update.refused_title', { version: found.version })}>
        {found.refused === 'package'
          ? t('app.update.refused_package', { version: found.version, current })
          : t('app.update.refused_manifest', { current })}
        {#snippet action()}
          <Button size="sm" tone="secondary" icon="external-link" onclick={() => appUpdate.open(RELEASES)}>
            {t('app.update.open_releases')}
          </Button>
          <Button size="sm" tone="subtle" onclick={() => appUpdate.later()}>{t('app.update.close')}</Button>
        {/snippet}
      </Banner>
    {/if}
  </div>
{/if}

<style>
  .update { margin-block-end: var(--sp-200); }

  /* The preview's link beside the buttons: the brand's colour, as a link reads in the banner. */
  .link {
    align-self: center;
    color: var(--text-brand);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
    text-decoration: none;
  }

  .link:hover { text-decoration: underline; }

  .link:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }
</style>
