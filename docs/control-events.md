# Durable control events

Migration 0017 stores per-recipient issued/acknowledged counters and immutable
protobuf outbox records. It is tested only in isolated databases, not deployed.

`controlevents.PublishTx` is a trusted service operation used inside the domain
transaction. Assignment creation and majority-answer acceptance now publish their
executor messages atomically with their records. A scoped event key deduplicates
identical retries; changed payload conflicts. Per-recipient row locks serialize
sequence allocation and transaction commits. A rollback leaves no event or gap.
Offline registered runners retain their events; transmission grants no authority.

`Page` force-verifies current runner/project access and binds the repository and
credential mapping under locks. It starts after the durable server ACK, returns
at most 32 events and 192 KiB, with payloads limited to 128 KiB each. Reading or
sending does not advance acknowledgement. `Acknowledge` requires the exact next
sequence and event UUID; skip-ahead acknowledgements fail. Repeated old/exact
acknowledgements are harmless. Another scoped identity cannot access the journal.

On Darwin/Linux, `State.PersistDelivery` validates contiguous sequences, retains
payloads and fingerprints, and returns an ACK only after private file/directory
fsync. Journals use the state directory's no-follow/ownership/0600 protections.
Restart restores unapplied events. Changed replay payloads, cursor gaps and reused
IDs fail. At 16 unapplied events the client refuses another ACK until processing
frees space. Applied payloads compact while up to 128 replay fingerprints remain.
`MarkDeliveryApplied` must follow a durable domain transition, not receipt alone.

Real-PG/race tests cover rollback, concurrent ordering, idempotent publication,
changed keys, replay after send/restart, exact/skip/duplicate ACKs, immutable history
and revoked access. Client tests cover private restart recovery, replay conflicts,
applied-event compaction and backpressure/closed-journal failure without ACK.
Domain tests verify assignment and exact-session answer messages commit together.

Task 5.2 remains open: council/stop notifications, native backend stream wiring,
client receive/ACK orchestration and deduplicated native execution are not fully
integrated. These storage tests do not establish end-to-end model delivery or
actual provider resume. The existing identity-only native backend stays closed
to work dispatch until its operational handler is implemented.
