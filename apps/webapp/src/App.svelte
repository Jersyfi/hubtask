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
  import { session } from './lib/session.svelte.ts';
  import ContainerView from './views/ContainerView.svelte';
  import HomeView from './views/HomeView.svelte';
  import ItemView from './views/ItemView.svelte';
  import InstallationView from './views/InstallationView.svelte';
  import SearchView from './views/SearchView.svelte';
  import TrashView from './views/TrashView.svelte';
  import SignInView from './views/SignInView.svelte';

  // A hub and a collection each have their own path, so a deep link to either survives a reload —
  // which is what ADR-0028's `index.html` fallback exists for. Two patterns rather than one
  // `/containers/:id`, because the two are different things to a reader and the address should say
  // which they are looking at.
  const router = new Router([
    { name: 'home', pattern: '/' },
    { name: 'installation', pattern: '/installation' },
    // No parameter, and that is the point: `/search` is a `POST` because a search term is content
    // and a query string travels through access logs, proxies and browser history. A route that
    // carried the term would undo that in the address bar (security.md §9, ADR-0018).
    { name: 'search', pattern: '/search' },
    { name: 'trash', pattern: '/trash' },
    // The address the board's cards and the search results have linked to since F2-11. An entry
    // is a thing with its own history (F2-15), so it is a screen rather than a row somewhere.
    { name: 'item', pattern: '/items/:id' },
    { name: 'hub', pattern: '/hubs/:id' },
    { name: 'collection', pattern: '/collections/:id' },
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

  // Signing in again returns the reader to what they were looking at when the session ended. The
  // path is taken once: one that navigated twice would fight the reader's next click.
  $effect(() => {
    if (!session.isSignedIn) return;
    const intended = session.takeIntendedPath();
    if (intended && intended !== route.path) router.navigate(intended);
  });
</script>

<AppFrame {route} onnavigate={(path) => router.navigate(path)}>
  <!-- Nothing here is usable without a credential, and the token screen is what asks for one. The
       route is left alone while it is shown, so that the address the reader arrived at is still
       the address they land on afterwards. -->
  {#if !session.isSignedIn}
    <SignInView />
  {:else if route.name === 'home'}
    <HomeView />
  {:else if route.name === 'installation'}
    <InstallationView />
  {:else if route.name === 'search'}
    <SearchView />
  {:else if route.name === 'trash'}
    <TrashView />
  {:else if route.name === 'item'}
    {#key route.params.id}
      <ItemView id={route.params.id ?? ''} />
    {/key}
  {:else if route.name === 'hub' || route.name === 'collection'}
    <!-- One view for both: they differ in what they hold, not in what they are. Keyed on the id so
         that navigating from one collection to another rebuilds rather than reusing the state of
         the one before — a draft rename would otherwise follow the reader to a different name. -->
    {#key route.params.id}
      <ContainerView id={route.params.id ?? ''} onnavigate={(path) => router.navigate(path)} />
    {/key}
  {:else}
    <!-- The server's own code for a path that reaches nothing, at the same address. -->
    <p>{t('route.unknown')} <code>{route.path}</code></p>
    <p><a href="/">{t('app.back_to_start')}</a></p>
  {/if}
</AppFrame>
