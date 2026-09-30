---
id: UC-INS-17
title: Run Hubtask on the hardware I have
context: admin
actors: [PE-selfhoster, PE-operator]
deployments: [D1, D2, D3, D4, D7]
serves: [P-09, P-10, P-11]
state: built
tasks: [B-15]
checked_by: [docs/architecture/support-matrix.md, .github/workflows/nightly.yml]
---

# Run Hubtask on the hardware I have

## Goal

A person runs Hubtask on whatever they already own — a small x86 server, a Raspberry Pi, a NAS with
an ARM processor, a Mac — and can trust that what the project says is supported was actually run
there, every night; and where support is only "best effort", the page says so before they buy
anything.

## Story

A family's technical parent has a Raspberry Pi 5 on the shelf. The support matrix says Docker on
`linux/arm64` is supported and names the nightly job that proves it. They run the same
`docker compose up` as on a PC and get the same product. A company's platform team reads that
Kubernetes on ARM is best effort and plans accordingly.

## How to check

1. The published image runs on `linux/amd64` and `linux/arm64` from one tag; `docker compose up`
   is the same on both.
2. Every row of `docs/architecture/support-matrix.md` marked *supported* names the CI job that
   proves it, and every such job exists; the doc gate refuses a row without its job and a job
   without its row.
3. The full test suite runs natively on arm64 every night; when it fails, an issue is opened, and it
   is closed by the next green run.
4. `hubctl` is published for Linux, macOS and Windows on both architectures, and the supported ones
   are run in CI.
5. A row that is only *best effort* says why, and no page claims more.

## Where it ends

* No 32-bit ARM, no RISC-V.
* Performance figures per board are not promised (the performance decision of 0.6.0).
