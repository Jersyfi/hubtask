<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The root: the route table, and the frame every view is rendered inside.
  //
  // The table is real paths over the History API rather than `#/` (ADR-0028): the server's
  // `index.html` fallback exists so that a deep link survives a reload, and a fragment would make
  // that fallback pointless. Past the fallback the application owns its own paths, which is why an
  // unknown one renders the not-found view below rather than a 404.
  //
  // No sentence is written in this file. `Hubtask` is a name rather than a message, and everything
  // else is a code rendered from `locales/en.json` (ADR-0011, F1-07).
  import AppFrame from './lib/frame/AppFrame.svelte';
  import { t } from './lib/i18n/i18n.svelte.ts';
  import { Router, type Resolution } from './lib/router.ts';
  import HomeView from './views/HomeView.svelte';
  import InstallationView from './views/InstallationView.svelte';

  const router = new Router([
    { name: 'home', pattern: '/' },
    { name: 'installation', pattern: '/installation' },
  ]);
  let route = $state<Resolution>(router.current);

  $effect(() => {
    const unsubscribe = router.subscribe((resolution) => (route = resolution));
    const stop = router.start();
    return () => {
      unsubscribe();
      stop();
    };
  });
</script>

<AppFrame {route}>
  {#if route.name === 'home'}
    <HomeView />
  {:else if route.name === 'installation'}
    <InstallationView />
  {:else}
    <!-- The server's own code for a path that reaches nothing, at the same address. -->
    <p>{t('route.unknown')} <code>{route.path}</code></p>
    <p><a href="/">{t('app.back_to_start')}</a></p>
  {/if}
</AppFrame>
