# RB-A38 — The mailbox watch is full

**Alert**: `HubtaskMailboxWatchFull` — `hubtask_mailbox_watches_unheld > 0` for an hour, or
`HubtaskMailboxWatchShardUnled` — fewer shards led (`hubtask_mailbox_watch_shards_led`, summed) than
`hubtask_mailbox_watch_shards` for half an hour. Severity `ticket`: nothing is lost, mail is slower
for some workspaces.

## What is happening

A worker of the installation holds the connected IMAP mailboxes open (IDLE), so that mail
reaches the desk within seconds (UC-JUM-11 check 10): one worker per shard of the workspaces
(`HUBTASK_MAILBOX_WATCH_SHARDS`, default one), each leading at most
`HUBTASK_MAILBOX_WATCH_SHARDS_PER_WORKER` shards. A shard's worker holds at most
`HUBTASK_MAILBOX_WATCH_LIMIT` of its mailboxes (default 1000), one per workspace before any
workspace's second. The gauge counts the mailboxes beyond that: they are still read every minute, but their
workspaces no longer hear of an arrival at once.

## What to do

1. Read `hubtask_mailbox_watches_held` beside it: the watch holds the limit, and the worker
   holding the watch's lock carries one TLS session per held mailbox.
2. If that worker has the connections and the memory to spare, raise `HUBTASK_MAILBOX_WATCH_LIMIT`
   on the worker role and roll it out; the alert clears on the next minute's tick.
3. If it does not, split the watch: raise `HUBTASK_MAILBOX_WATCH_SHARDS` and set
   `HUBTASK_MAILBOX_WATCH_SHARDS_PER_WORKER` so the shards spread over the workers, with one worker
   to spare (four shards, three workers: two each). Otherwise leave the limit: every workspace
   keeps at least one held mailbox, and the rest keep the minute's read.
4. For a shard without a worker: a worker stopped, or the workers times the shards per worker are
   fewer than the shards. Start the worker again, or raise the shards per worker; a worker with a
   seat free takes the shard on its next minute's tick.
