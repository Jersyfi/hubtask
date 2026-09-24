# ADR-0067 — The S3-compatible server the tests run against is SeaweedFS, in one place

**Status:** accepted (2026-09-24) · **Date:** 2026-09-24

## Context

Four tests need a real S3 server, because four claims cannot be made against a fake:

| Test | The claim |
|---|---|
| `TestObjectStoreConformance/s3` | The S3 adapter answers the port the local one answers |
| `TestAPresignedURLWorksUntilItExpires` | The upload window is enforced **by storage**, not by this server |
| `TestBackupTargetConformance/s3` | The S3 backup target, beside local, WebDAV and SFTP |
| `TestRT1AStoppedContainerDegradesExactlyItsOwnFeature` | A stopped object store degrades exactly its own feature |

The first of those carries more than it looks. The SigV4 signer is hand-written
([`infrastructure/awssig`](../../infrastructure/awssig/SigV4.go)); its unit tests prove the
signing-key derivation against the vector Amazon publishes and the canonical forms, and its own
header says the rest: *"The full signature is proved by MinIO in the conformance suite — MinIO
validates strictly."* One container is the only end-to-end evidence that the signer is right.

That container stopped being available. **MinIO archived its open-source server, client and KES**:

* `github.com/minio/minio` and `github.com/minio/mc` are archived.
* `dl.min.io` answers **410 Gone** to every path, with a body saying there will be no product
  support, no security updates and no security advisories, and that vulnerability reports are not
  accepted.
* Docker Hub stopped serving `minio/minio` anonymously on 2026-09-11; the quay.io fallback that
  replaced it closed on 2026-09-24 (#1029). Both answer 401.

Three pull-request gates went red on every branch, on no diff: Integration (PostgreSQL), Data gates
and Resilience RT-1..RT-12, and with them `CI required`.

The pin existed in **five places**: three copies of the same helper in three test packages, and two
more in `scripts/pitr-drill.sh` — the server and `mc`. That is why a vendor's decision became a
five-place problem rather than a one-line one.

## Decision

**The S3-compatible server is SeaweedFS, pinned in one place: `test/s3test`.**

1. `test/s3test` starts the server for every suite that needs one, the way `test/dbtest` starts the
   PostgreSQL. It owns the pin, the credential, the region, the readiness probe and the bucket
   creation. `scripts/pitr-drill.sh` reads the same default through `HUBTASK_TEST_S3_IMAGE`.
2. The pin is `chrislusf/seaweedfs:4.47`, overridable by `HUBTASK_TEST_S3_IMAGE` — the variable
   `HUBTASK_TEST_MINIO_IMAGE` used to be, renamed because it no longer names what it starts.
3. `test/s3test` has its own suite. A helper that silently does nothing is worse than no helper,
   and two of SeaweedFS's behaviours make that a live risk rather than a theoretical one — see
   *What the helper has to defend against* below.

### Why SeaweedFS, measured rather than assumed

Every candidate was run against the assertions these suites actually make, with a control — a wrong
secret key must be refused, or "all green" could equally mean "the server never checks":

| | MinIO (baseline) | **SeaweedFS 4.47** | RustFS | Garage 2.0 |
|---|---|---|---|---|
| Round trip, content type, size | yes | yes | yes | yes |
| 8 MiB stream, byte for byte | yes | yes | yes | yes |
| Deletion complete and idempotent | yes | yes | yes | yes |
| Expired presigned PUT | **403** `Request has expired` | **403** `Request has expired` | 403 | 400 `Date is too old` |
| Tampered signature | **403** `SignatureDoesNotMatch` | **403** `SignatureDoesNotMatch` | 400 | 403 |
| Wrong secret key on a header-signed call | refused | refused | refused | refused |
| `response-content-disposition` honoured | yes | yes | yes | yes |
| **Result** | 8/8 | **8/8, same status codes** | 7/8 | 7/8 |
| Bootstrap | 2 environment variables | **1 JSON file** | 2 environment variables | 7 commands |
| Licence | AGPL-3.0, archived | **Apache-2.0** | Apache-2.0 | AGPL-3.0 |
| Upstream | none | weekly releases since 2014 | `preview` releases only | stable |

SeaweedFS is the only candidate that answers with the **same status codes**, which is why no
assertion in the suites had to be relaxed — the change is the server, not what is asserted of it.
Garage and RustFS enforce the same rules in a different dialect; adopting either would have meant
rewriting an assertion at the same time as the thing it asserts about, and RustFS has no stable
release to pin. Garage additionally needs `layout assign`, `layout apply`, `key import`,
`key allow`, `bucket create` and `bucket allow` before it serves a request — in three test packages
and a shell script.

### Licence and cost

Apache-2.0, in an unmodified `LICENSE`, with **no CLA** in the repository and no `NOTICE` file. No
fee, no registration, no key, no usage limit. There is a SeaweedFS Enterprise Edition; nothing
these suites assert lives behind it.

Running a container in a test is neither distribution nor linking: `weed` enters no Hubtask binary
and no Hubtask image, Hubtask's own BUSL-1.1 is untouched, and this does not belong in
`THIRD-PARTY-LICENSES.md`, which `go-licenses` derives from Go modules. Should the image ever be
mirrored, Apache-2.0 §4 asks only that the licence and the notices travel with it — the file is
already in the image layers, and §4(d) does not apply without a `NOTICE`. AGPL-3.0 would have made
that same mirror a question about offering corresponding source.

### The risks this accepts

* **Bus factor.** `chrislusf` has 10,496 commits; the next human has 391.
* **The same structure MinIO had.** An open-source edition beside a commercial one. What is
  different is that Apache-2.0 without a CLA leaves the copyright distributed, so the existing
  releases cannot be relicensed away, and 3,005 forks may continue them.
* **The working registry path is a personal namespace.** `chrislusf/seaweedfs` serves;
  `seaweedfs/seaweedfs` on Docker Hub and `ghcr.io/seaweedfs/seaweedfs` do not.

These are tolerable because this is a **test fixture**. No user data touches it, it is never
exposed beyond loopback, and if SeaweedFS disappears the cost is another half-day — not a
migration. The measurement above is reproducible against any successor.

**No licence prevents a vendor from deleting its images.** Apache-2.0 does not make SeaweedFS
safer to depend on than MinIO was; it makes our own copy lawful and cheap to keep. That copy is the
next decision, not this one.

## What the helper has to defend against

**Four ways to report a success that did not happen.** All four were found by trying rather than by
reading, and none of them announces itself — each one ends in a green step with no bucket:

* **`weed shell` exits 0 on a command it does not know**, printing the complaint instead. So
  `s3test.CreateBucket` reads the listing back rather than trusting the exit code.
* **It blocks indefinitely when it cannot reach the master** rather than failing. So "the call
  returned" is not evidence either: the Go helper carries its own 30-second deadline, independent
  of the caller's context.
* **`container.Exec` hands back Docker's multiplexed attach stream.** Its eight-byte frame headers
  land *inside* the text, where they read as a shell prompt glued to the first line
  (`/  written-immediately`), so any check that looks at *where* a word sits breaks. The reader
  passes `tcexec.Multiplexed()`. A lenient `strings.Contains` had hidden this completely — it found
  the name despite the frame bytes. The strict whole-name check is what surfaced it.
* **`weed shell` is a gRPC client on `port + 10000`.** It is *given* the master's HTTP port, 9333,
  but does its work on **19333**, and the filer's on **18888**. A Job beside the server, talking to
  a Service that published 8333 and 9333 only, exited **0 having printed nothing at all** — no
  error, no output, no bucket — and then sat until its deadline. So the drill makes its buckets
  **inside the object store's own pod**, where every port is on localhost. Publishing the two gRPC
  ports would also have worked and was rejected deliberately: it would tie the drill to an
  arithmetic SeaweedFS is free to change.

The last of those was found only by running `gate-pitr` in full, on a real cluster — the one part
of this change no pull-request gate reaches. It is the argument for running it before merging
rather than discovering it in a nightly.

**The shape of the defence is one rule**: read the result back through a second channel, and match
the whole name. `s3.bucket.create` then `s3.bucket.list`, and the bucket counts as made only if the
listing names it exactly — `media` must not be found in a listing that holds only `hubtask-media`.
A loose assertion inside a helper hides bugs in the helper, and the helper is what every suite
trusts.

## Consequences

* The three red gates go green with no assertion changed, no fake introduced, and nothing skipped.
* The next registry or vendor move is one line in `test/s3test` plus one default in the drill.
* `test/s3test` has its own suite, in `gate-integration`: a container helper that silently does
  nothing takes every suite that leans on it down with it, green. Three of the four traps above
  have a test that triggers them.
* `HUBTASK_TEST_MINIO_IMAGE` no longer exists. No workflow referenced it. The RT-1 evidence records
  under `docs/evidence/` still name it and are left alone — they are records of what ran then.
* The suite names change from `s3 against MinIO` to `s3 against SeaweedFS`.
* Documentation that pointed operators at MinIO as an example S3 service is corrected, because it
  now points at archived software with no security updates. [ADR-0016](./ADR-0016-observability-reliability.md)
  and [ADR-0047](./ADR-0047-media-origin-in-the-interface-policy.md) mention MinIO in passing; they
  are accepted and are not reopened here.

## What this does not decide

* **Where the image lives.** Mirroring the pinned image into this project's own registry is the
  durable answer to a third party closing a door, and it is independent of which server is pinned.
  Apache-2.0 makes it cheap; it is a separate decision with its own ADR.
* **What Hubtask itself recommends to operators.** The product supports any S3-compatible service
  and takes no position on which one an operator runs.

## Alternatives considered

* **Mirror the pinned MinIO release into our own registry.** The exact bytes, no test change. It
  fails on the arithmetic: only `linux/arm64` is still obtainable from a local cache, `dl.min.io`
  is 410 and Docker Hub and quay.io are 401, so the amd64 image CI needs cannot be reconstructed.
  The GitHub releases of the archived repository do still carry both binaries, so an image could be
  *built* from them — but that pins CI to software its authors say receives no security updates and
  no advisories, forever, and publishing an AGPL-3.0 binary image raises a source-offer question a
  test fixture should not have to answer.
* **Authenticate to quay.io with a robot account.** `quay.io/minio` is MinIO's organisation; there
  is no access to grant ourselves, and the project is archived.
* **A fake, or a lax mock (`adobe/s3mock`, LocalStack).** Both would answer every request the
  adapter makes and validate none of the signatures, which turns the one end-to-end proof of the
  hand-written signer into a test that agrees with itself.
