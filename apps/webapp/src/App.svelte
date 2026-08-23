<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The root of the to-do application. Still a shell on purpose: W-06 scaffolds the framework
  // and nothing more — the first view, the first API call, the first component all belong to
  // later tasks. What already holds here is what will hold everywhere: every visual value is a
  // token (ADR-0029), and component styles compile to the external stylesheet the CSP of
  // ADR-0028 requires.
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
    <p>The application shell is scaffolded: Svelte 5 as a plain Vite SPA (ADR-0030).</p>
  {:else}
    <h1>Hubtask</h1>
    <p>There is nothing at <code>{route.path}</code>. <a href="/">Back to the start.</a></p>
  {/if}
</main>
