<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A conversation as a caller holds it: sentences already resolved, and the two affordances
  // switched on per comment, because whether this reader is the author is the caller's knowledge.

  import CommentThread, { type ThreadComment } from './CommentThread.svelte';

  const { mode = 'thread' }: { mode?: 'thread' | 'removed' | 'empty' | 'paged' } = $props();

  const conversation: ThreadComment[] = [
    {
      id: 'c1',
      authorName: 'Amelie Fischer',
      body: 'The delivery slot moved to Thursday. Does that still work for the kitchen?',
      when: '4 Sep 2026, 09:12',
      at: '2026-09-04T07:12:00Z',
      // This reader wrote it, so both affordances are on.
      replies: [
        {
          id: 'c1r1',
          authorName: 'Jonas Brandt',
          body: 'Thursday is fine. I will move the fitters to the afternoon.',
          when: '4 Sep 2026, 09:31',
          at: '2026-09-04T07:31:00Z',
          editDisabledReason: 'Only the author can change a comment.',
          deleteDisabledReason: 'Only the author or an administrator can remove a comment.',
        },
      ],
    },
    {
      id: 'c2',
      authorName: 'Sofia Marchetti',
      body: 'I have attached the revised plan to the work package.',
      when: '4 Sep 2026, 11:48',
      at: '2026-09-04T09:48:00Z',
      editedLabel: 'edited',
      editDisabledReason: 'Only the author can change a comment.',
      deleteDisabledReason: 'Only the author or an administrator can remove a comment.',
    },
  ];

  const withRemoved: ThreadComment[] = [
    {
      id: 'r1',
      authorName: 'Tobias Lang',
      isRemoved: true,
      when: '3 Sep 2026, 16:02',
      at: '2026-09-03T14:02:00Z',
      replies: [
        {
          id: 'r1r1',
          authorName: 'Amelie Fischer',
          body: 'Agreed — I have raised it with the supplier.',
          when: '3 Sep 2026, 16:20',
          at: '2026-09-03T14:20:00Z',
          editDisabledReason: 'Only the author can change a comment.',
          deleteDisabledReason: 'Only the author or an administrator can remove a comment.',
        },
      ],
    },
  ];
</script>

<CommentThread
  label="Comments"
  comments={mode === 'empty' ? [] : mode === 'removed' ? withRemoved : conversation}
  emptyLabel="Nothing has been said about this entry yet. Start the conversation."
  removedLabel="This comment was removed."
  editLabel="Edit"
  deleteLabel="Remove"
  replyLabel="Reply"
  hasMore={mode === 'paged'}
  loadMoreLabel="Older comments"
  arrivedLabel="8 older comments"
/>
