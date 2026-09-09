<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What each role may do, as **this installation** enforces it.
  //
  // **A table of what the server says, not a control.** Nothing in it is editable, because changing
  // what a role carries is not an operation this product has: a role is granted, and the matrix
  // says what the grant means. A component with a switch in it would imply a call nobody can make.
  //
  // **Nothing is compiled in.** The roles, the permission columns and the wording of every cell are
  // handed in, because `/meta/capabilities` answers the matrix and the contract says in its own
  // words that "a client that does not know them offers buttons the server refuses". A role this
  // component has never heard of gets a row; a permission it has never heard of gets a column; a
  // qualifier it cannot word is shown as the server's own token rather than hidden, because
  // hiding it would tell the reader that the cell means nothing when it means something unread.
  //
  // **The qualifiers are columns, not ticks.** `ASSIGNED` — "only where the actor is the entry's
  // assignee" — is not a weaker yes, and a tick that stood for it would be a lie a reader cannot
  // see. So `item_access` is rendered in words beside the permission columns: `read`, `create`,
  // `change` and `comment`, each answered for every role, including the ones that are `NONE`.

  import Icon from './Icon.svelte';
  import Table, { type Column } from './Table.svelte';
  import VisuallyHidden from './VisuallyHidden.svelte';

  /** One row: a role, what it carries, and how far it reaches into one entry. */
  export interface MatrixRole {
    /** The role as the manifest names it. The key, never drawn. */
    readonly id: string;
    /** The role's name, resolved (ADR-0011). */
    readonly label: string;
    /** The permission ids this role carries unqualified. */
    readonly permissions: readonly string[];
    /**
     * How far it reaches into one entry, per kind of access — the server's own token per kind
     * (`ALL`, `ASSIGNED`, `NONE`, or one this build has never seen).
     */
    readonly itemAccess?: Readonly<Record<string, string>>;
  }

  interface Props {
    /** What the table is. Becomes its caption. */
    label: string;
    roles: readonly MatrixRole[];
    /** The heading of the first column — "Role". */
    roleColumnLabel: string;
    /** The permission columns, in the order they are read. */
    permissions: readonly Column[];
    /** The `item_access` columns: read, create, change, comment. */
    accessKinds?: readonly Column[];
    /**
     * How each `item_access` token is worded. A token that is not in here is drawn as itself —
     * the manifest reporting something this build does not know is a fact worth showing, and an
     * empty cell would read as "nothing".
     */
    accessLabels?: Readonly<Record<string, string>>;
    /** What a tick means, and what its absence means. Announced, never drawn. */
    carriedLabel: string;
    notCarriedLabel: string;
  }

  const {
    label,
    roles,
    roleColumnLabel,
    permissions,
    accessKinds = [],
    accessLabels = {},
    carriedLabel,
    notCarriedLabel,
  }: Props = $props();

  const columns = $derived<readonly Column[]>([
    { id: '__role', label: roleColumnLabel },
    ...permissions,
    ...accessKinds,
  ]);

  const carries = (role: MatrixRole, permission: string) => role.permissions.includes(permission);
</script>

<Table {label} {columns}>
  {#each roles as role (role.id)}
    <tr>
      <!-- `scope="row"`: reading across a row of ticks is the whole use of this table, and a cell
           that is not a header for its row leaves a screen reader announcing "yes" with no subject. -->
      <th scope="row" class="role">{role.label}</th>

      {#each permissions as permission (permission.id)}
        <td class="mark">
          <!-- Rule 3: the mark never stands alone. A tick and a dash survive greyscale, print and
               colour vision deficiency; the word beside each is what a screen reader reads, and it
               is the caller's word rather than one written here. -->
          {#if carries(role, permission.id)}
            <Icon name="check" size="sm" />
            <VisuallyHidden>{carriedLabel}</VisuallyHidden>
          {:else}
            <Icon name="minus" size="sm" />
            <VisuallyHidden>{notCarriedLabel}</VisuallyHidden>
          {/if}
        </td>
      {/each}

      {#each accessKinds as kind (kind.id)}
        {@const token = role.itemAccess?.[kind.id]}
        <td class="access">
          {#if token === undefined}
            <!-- The manifest answered no value for this kind. Not "no access": unanswered. -->
            <Icon name="minus" size="sm" />
            <VisuallyHidden>{notCarriedLabel}</VisuallyHidden>
          {:else}
            {accessLabels[token] ?? token}
          {/if}
        </td>
      {/each}
    </tr>
  {/each}
</Table>

<style>
  .role { color: var(--text-primary); font-weight: var(--fw-medium); white-space: nowrap; }

  .mark { color: var(--text-secondary); }

  .access { color: var(--text-secondary); white-space: nowrap; }
</style>
