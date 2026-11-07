<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The three rungs of the dialog ladder with the mockup's own examples, and the discard
  // confirmation over a changed one.
  import Button from './Button.svelte';
  import Checkbox from './Checkbox.svelte';
  import Dialog from './Dialog.svelte';
  import Input from './Input.svelte';
  import Inline from './Inline.svelte';
  import SearchField from './SearchField.svelte';
  import Select from './Select.svelte';
  import Stack from './Stack.svelte';
  import Textarea from './Textarea.svelte';

  const { size = 'm' }: { size?: 's' | 'm' | 'l' | 'discard' } = $props();

  let isOpen = $state(false);
  let isAsking = $state(false);
  let name = $state('Change Advisory Board');
  const saved = 'Change Advisory Board';

  const people = ['Amelia Hughes', 'Mehmet Yılmaz', 'Sofia Rossi', 'Jonas Weber', 'Clara Neumann', 'Thomas Becker', 'Priya Natarajan', 'Lukas Brandt'];
  const roles = [
    { value: 'member', label: 'Member' },
    { value: 'admin', label: 'Administrator' },
    { value: 'viewer', label: 'Viewer' },
  ];
</script>

<Stack gap="200" align="start">
  <Button tone={size === 's' ? 'danger' : 'primary'} onclick={() => (isOpen = true)}>
    {size === 's' ? 'Delete group…' : size === 'l' ? 'Add people to the group' : size === 'discard' ? 'Edit group' : 'Add people'}
  </Button>
</Stack>

{#if size === 's'}
  <Dialog bind:isOpen size="s" title="Delete “Change Advisory Board”?" primary={{ label: 'Delete group', tone: 'danger', onclick: () => (isOpen = false) }}>
    Its 2 people keep their own roles. The role “CAB member” it grants ends for them. This cannot be undone.
    {#snippet actions()}<Button tone="subtle" onclick={() => (isOpen = false)}>Cancel</Button>{/snippet}
  </Dialog>
{:else if size === 'l'}
  <Dialog bind:isOpen size="l" title="Add people to Change Advisory Board" dismissLabel="Close" primary={{ label: 'Add 2 people', onclick: () => (isOpen = false) }}>
    <SearchField label="Search people and groups" isLabelHidden clearLabel="Clear the search" />
    <Stack gap="100">
      {#each people as person, index (person)}
        <Checkbox label={person} checked={index < 2} />
      {/each}
    </Stack>
    {#snippet actions()}<Button tone="subtle" onclick={() => (isOpen = false)}>Cancel</Button>{/snippet}
  </Dialog>
{:else if size === 'discard'}
  <Dialog
    bind:isOpen
    size="m"
    title="Edit group"
    dismissLabel="Close"
    primary={{ label: 'Save', onclick: () => (isOpen = false) }}
    onDismiss={() => {
      if (name === saved) return true;
      isAsking = true;
      return false;
    }}
  >
    <Input label="Name" bind:value={name} isRequired />
    <Textarea label="Description" value="Approves normal and emergency changes" hint="Shown under the name in lists." />
    {#snippet actions()}<Button tone="subtle" onclick={() => (isOpen = false)}>Cancel</Button>{/snippet}
  </Dialog>
  <Dialog bind:isOpen={isAsking} size="s" title="Discard changes?" primary={{ label: 'Discard', tone: 'danger', onclick: () => { isAsking = false; name = saved; isOpen = false; } }}>
    The name you changed is not saved.
    {#snippet actions()}<Button tone="subtle" onclick={() => (isAsking = false)}>Keep editing</Button>{/snippet}
  </Dialog>
{:else}
  <Dialog bind:isOpen size="m" title="Add people" dismissLabel="Close" primary={{ label: 'Send invitations', onclick: () => (isOpen = false) }}>
    <Input label="E-mail addresses" isRequired value="anna@nordlicht.example.org, ben@nordlicht.example.org" hint="Several at once, separated by commas." />
    <Inline gap="200"><Select label="Role in the workspace" options={roles} value="member" /></Inline>
    <Input label="Groups" placeholder="Add to groups…" />
    {#snippet actions()}<Button tone="subtle" onclick={() => (isOpen = false)}>Cancel</Button>{/snippet}
  </Dialog>
{/if}
