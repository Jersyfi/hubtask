<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What the Android app shows where a screen of the workspace's administration or the
  // installation's area would be (ADR-0032, UC-SYN-07 check 7): the build leaves both areas out
  // (`platform/target.ts` puts this page in their modules' place), and a link into one that reaches
  // the app - a notification, a shared link - says where it lives and opens the same address in the
  // web app of the server the app is signed in to. The browser's session there is the person's own.
  import { Button, PageHeader, Stack } from '@hubtask/design-system/components';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { platform } from '../lib/platform/index.ts';
  import type { Resolution } from '../lib/router.ts';
  import { webAppAddress } from '../lib/webApp.ts';

  interface Props {
    route: Resolution;
    onnavigate: (path: string) => void;
    onreplace: (path: string) => void;
  }

  const { route }: Props = $props();

  const isInstance = $derived(route.area === 'instance');
  const title = $derived(t(isInstance ? 'app.nav.instance' : 'app.nav.administration'));
  const query = $derived(new URLSearchParams(route.query).toString());
  const address = $derived(
    webAppAddress(route.area, query === '' ? route.path : `${route.path}?${query}`, {
      workspace: platform.webOrigin,
      installation: manifest.value?.installation_url,
    }),
  );
  $effect(() => page.entitle(title));

  function open() {
    void platform.webApp?.open(address.href).catch(() => {});
  }
</script>

<Stack gap="300">
  <PageHeader {title} isTitleInBar={viewport.isCompact} />
  <section class="panel">
    <Stack gap="100">
      <h2>{t('app.web_app.title')}</h2>
      <p class="prose">{t(isInstance ? 'app.web_app.body_instance' : 'app.web_app.body_administration', { area: title })}</p>
      <p class="fixed">{address.host}{address.pathname}{address.search}</p>
      <Button tone="primary" icon="external-link" isFull onclick={open}>{t('app.web_app.open')}</Button>
    </Stack>
  </section>
</Stack>

<style>
  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
    max-inline-size: 72ch;
  }

  h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
  }

  p {
    margin: 0;
  }

  .prose {
    color: var(--text-secondary);
  }

  .fixed {
    font-family: var(--font-mono);
    font-size: var(--fs-100);
    font-weight: var(--fw-regular);
    color: var(--text-primary);
    overflow-wrap: anywhere;
  }
</style>
