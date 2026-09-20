<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import AppBar from './AppBar.svelte';
  import Avatar from './Avatar.svelte';
  import IconButton from './IconButton.svelte';
  import Menu from './Menu.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'brand' }: { mode?: 'brand' | 'title' | 'rail' | 'long' } = $props();

  let isOpen = $state(false);
  let isPinned = $state(true);
  let chosen = $state<string | undefined>(undefined);

  // The account group of the one navigation list (ADR-0061 decision 1): what the avatar opens.
  const account = [
    { id: 'settings', label: 'Your settings', icon: 'settings' as const },
    { id: 'installation', label: 'This installation', icon: 'info' as const },
    { id: 'tour', label: 'Take the tour again', icon: 'info' as const },
    { id: 'sign-out', label: 'Sign out', icon: 'log-out' as const },
  ];

  const title = $derived(
    mode === 'long' ? 'Vorbereitung der Jahreshauptversammlung mit Rechenschaftsbericht' : 'Kitchen',
  );
</script>

<Stack gap="200">
  {#if mode === 'brand'}
    <AppBar
      label="Application"
      toggle={{ kind: 'drawer', label: 'Open the navigation', isExpanded: isOpen, onToggle: () => (isOpen = !isOpen) }}
    >
      {#snippet brand()}
        <a class="wordmark" href="/">Hubtask</a>
      {/snippet}
      {#snippet end()}
        <Menu label="You" items={account} placement={{ side: 'block-end', align: 'end' }} onselect={(id) => (chosen = id)}>
          {#snippet trigger(props)}
            <button type="button" class="avatar-button" {...props}>
              <Avatar name="Jérôme Winkel" size="sm" />
            </button>
          {/snippet}
        </Menu>
      {/snippet}
    </AppBar>
  {:else if mode === 'rail'}
    <AppBar
      label="Application"
      toggle={{ kind: 'rail', label: isPinned ? 'Collapse the navigation' : 'Expand the navigation', isExpanded: isPinned, onToggle: () => (isPinned = !isPinned) }}
    >
      {#snippet brand()}
        <a class="wordmark" href="/">Hubtask</a>
      {/snippet}
      {#snippet end()}
        <Avatar name="Jérôme Winkel" size="sm" />
      {/snippet}
    </AppBar>
  {:else}
    <AppBar
      label="Application"
      {title}
      toggle={{ kind: 'drawer', label: 'Open the navigation', isExpanded: isOpen, onToggle: () => (isOpen = !isOpen) }}
    >
      {#snippet end()}
        <IconButton icon="ellipsis" label="Page actions" />
      {/snippet}
    </AppBar>
  {/if}
  <p class="state">
    {#if mode === 'rail'}
      The navigation is {isPinned ? 'pinned' : 'a rail'}.
    {:else}
      The drawer is {isOpen ? 'open' : 'closed'}{chosen ? `; chosen: ${chosen}` : ''}.
    {/if}
  </p>
</Stack>

<style>
  .wordmark {
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    color: var(--text-primary);
    text-decoration: none;
  }

  .wordmark:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }

  .avatar-button {
    display: inline-flex;
    padding: 0;
    border: 0;
    border-radius: var(--r-full);
    background: transparent;
    cursor: pointer;
  }

  .avatar-button:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  .state { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }
</style>
