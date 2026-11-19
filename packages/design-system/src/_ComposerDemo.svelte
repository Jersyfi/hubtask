<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { untrack } from 'svelte';

  import Composer, { type ComposerMode } from './Composer.svelte';

  interface Props {
    mode?: 'ticket' | 'note' | 'comment' | 'readonly';
  }

  const { mode = 'ticket' }: Props = $props();

  let kind = $state<ComposerMode>(untrack(() => (mode === 'note' ? 'note' : 'reply')));
  let value = $state(untrack(() => (mode === 'note' ? 'Load balancer log shows 504 from both app servers; suspect the DB connection pool is exhausted.' : '')));
  let said = $state('');

  const macros = {
    label: 'Macros',
    title: 'Macros · run on SA-12',
    items: [
      { id: 'solve', label: 'Solve as answered', description: 'Status Solved · label password · reply from snippet “Solved”' },
      { id: 'second', label: 'Hand to 2nd level', description: 'Group 2nd level · internal note with your text' },
      { id: 'ask', label: 'Ask back', description: 'Status Waiting for requester · SLA paused' },
    ],
    onselect: (id: string) => (said = `Ran macro ${id}`),
  };
  const snippets = { label: 'Snippets', items: [{ id: 'greet', label: 'Greeting' }, { id: 'solved', label: 'Solved' }], onselect: (id: string) => (said = `Inserted ${id}`) };
</script>

<div class="column">
  {#if mode === 'comment'}
    <Composer label="Comment" placeholder="Comment … @ mentions, / inserts, drop files" sendLabel="Comment" hint="Ctrl+↵ sends" bind:value attach={{ label: 'Attach a file', onattach: () => {} }} format={{ label: 'Formatting', items: [{ id: 'bold', label: 'Bold', icon: 'bold' }, { id: 'italic', label: 'Italic', icon: 'italic' }, { id: 'list', label: 'List', icon: 'list' }, { id: 'link', label: 'Link', icon: 'link' }, { id: 'mention', label: 'Mention someone', icon: 'at-sign' }], onformat: (id) => (said = `Format ${id}`) }} onsend={(text) => { said = `Sent: ${text}`; value = ''; }} />
  {:else if mode === 'readonly'}
    <Composer label="Comment" sendLabel="Comment" disabledReason="Comments are closed: this story is archived." />
  {:else}
    <Composer
      label="Reply to Hannah Albers"
      placeholder="Reply …"
      modes={{ reply: 'Reply to Hannah Albers', note: 'Internal note', label: 'Reply or note' }}
      bind:mode={kind}
      bind:value
      audience="sees: Hannah, Stadtwerke Lüneburg (shares requests)"
      noteAudience="only people in the IT service desk see it"
      sendLabel="Send reply"
      noteSendLabel="Save note"
      hint="Ctrl+↵ sends"
      attach={{ label: 'Attach a file', onattach: () => {} }}
      {snippets}
      {macros}
      onsend={(text, as) => { said = `${as === 'note' ? 'Saved note' : 'Sent reply'}: ${text}`; value = ''; }}
    />
  {/if}
  <p class="said" role="status">{said}</p>
</div>

<style>
  .column { min-inline-size: 0; }
  .said { color: var(--text-subtle); font-size: var(--fs-075); }
</style>
