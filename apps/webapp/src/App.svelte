<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The root of the to-do application. Still a shell on purpose: W-06 scaffolds the framework
  // and nothing more — the first view, the first API call, the first component all belong to
  // later tasks. What already holds here is what will hold everywhere: every visual value is a
  // token (ADR-0029), component styles compile to the external stylesheet the CSP of ADR-0028
  // requires, and no sentence is written in this file — every string is a code rendered from
  // `locales/en.json` (ADR-0011, i18n-l10n.md §1). `Hubtask` is a name rather than a message,
  // which is why it is the one word here that is not a code.
  import { Button, Stack } from '@hubtask/design-system/components';

  import { t } from './lib/i18n/i18n.svelte.ts';
  import { Router, type Resolution } from './lib/router.ts';

  // One route. The table grows with the views; a path outside it renders the not-found shell
  // below rather than a 404, because past the index.html fallback (ADR-0028) the application
  // owns its own paths.
  const router = new Router([{ name: 'home', pattern: '/' }]);
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

<main>
  {#if route.name === 'home'}
    <h1>Hubtask</h1>
    <p>{t('app.shell.scaffolded')}</p>
    <!-- The first component from the design system to reach the bundle. It is here to be
         *checked*, not to be useful: `build/check-csp.js` reads the built output, and until
         something from packages/ was in it the promise that component styles compile to the
         external stylesheet was untested from this side. -->
    <Stack gap="200" as="section">
      <p>{t('app.shell.first_control')}</p>
      <Button tone="secondary" icon="check" disabledReason={t('app.shell.nothing_to_do')}>
        {t('app.shell.create_task')}
      </Button>
    </Stack>
  {:else}
    <h1>Hubtask</h1>
    <!-- The server's own code for this, rather than a second sentence saying the same thing: the
         router answers `route.unknown` when a request reaches no route, and the client is at the
         same address with the same answer. -->
    <p>{t('route.unknown')} <code>{route.path}</code></p>
    <p><a href="/">{t('app.back_to_start')}</a></p>
  {/if}
</main>
