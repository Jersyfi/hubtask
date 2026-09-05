#!/usr/bin/env python3
# SPDX-License-Identifier: BUSL-1.1
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# Reads a tenant export the way docs/architecture/tenant-export.md says a reader must, and says
# what it found (H-16).
#
# It is written against the document rather than against the writer, which is the whole point:
# "somebody must be able to build an importer against what is written here ... without reading
# Hubtask's source" (§2). So it imports nothing of this project, knows no Go type, and every check
# below names the section it comes from. If it disagrees with the writer, one of the two is wrong
# and the document decides which.
#
# Steps 1 and 2 of §8 - the archive is committed, and every member matches its digest - are the
# scripted session's own two lines, because `checksums.txt` is `sha256sum`'s format by design.
# What is left is the arithmetic: steps 3, 4 and 5.
#
#   verify-tenant-export.py <archive directory> <tenant uuid>

import hashlib
import json
import os
import sys

FORMAT_VERSION = 1


def main(root: str, tenant: str) -> int:
    problems: list[str] = []
    manifest_path = os.path.join(root, "manifest.json")
    if not os.path.isfile(manifest_path):
        print(f"verify-tenant-export: {manifest_path} is missing (§3)", file=sys.stderr)
        return 1
    with open(manifest_path, encoding="utf-8") as handle:
        manifest = json.load(handle)

    # §4: what an archive says it is. A reader "must refuse a version above the one they know",
    # and the other three are what a tenant export is by definition rather than by configuration.
    version = manifest.get("format_version")
    if version != FORMAT_VERSION:
        problems.append(f"format_version is {version!r}, and this reader knows {FORMAT_VERSION}")
    if manifest.get("mode") != "FULL":
        problems.append(f"mode is {manifest.get('mode')!r}; a tenant export is always FULL (§1)")
    encryption = (manifest.get("encryption") or {}).get("mode")
    if encryption != "NONE":
        problems.append(
            f"encryption is {encryption!r}; an export is never encrypted, which is the point (§1)"
        )
    scope = manifest.get("scope") or {}
    if scope.get("kind") != "TENANT" or scope.get("id") != tenant:
        problems.append(f"scope is {scope!r}, want the exported workspace (§4)")

    # §8.3: the manifest, the digests and the line counts have to agree with each other. A count
    # that matches nothing is how an archive loses records quietly.
    counts = manifest.get("counts") or {}
    files = manifest.get("files") or []
    if not files:
        problems.append("the manifest lists no data files (§4)")
    for entry in files:
        path = entry.get("path", "")
        member = os.path.join(root, path)
        if not os.path.isfile(member):
            problems.append(f"{path} is listed in the manifest and is not in the archive")
            continue
        with open(member, "rb") as handle:
            body = handle.read()
        digest = hashlib.sha256(body).hexdigest()
        if digest != entry.get("sha256"):
            problems.append(f"{path}: the manifest's digest is not the file's")
        if len(body) != entry.get("bytes"):
            problems.append(f"{path}: the manifest's size is not the file's")
        # Lines rather than newlines: a writer that ends the last record without one is still
        # writing one record per line, and counting separators would lose it.
        records = len(body.splitlines())
        # An absent `records` is nought. §4 lists the field on every entry and the writer omits it
        # when it is zero, which is most entries in most archives - an entity with no rows still
        # gets its file (§3). A reader that refused the omission would refuse nearly every archive
        # over a difference it can resolve; one that ignored a *wrong* count would be no reader at
        # all, which is why only the absence is forgiven.
        claimed = entry.get("records", 0) or 0
        if records != claimed:
            problems.append(f"{path}: {records} lines, and the manifest says {claimed}")
        entity = os.path.basename(path).removesuffix(".jsonl")
        if (counts.get(entity) or 0) != claimed:
            problems.append(f"{entity}: counts says {counts.get(entity)} and files says {claimed}")
        problems.extend(read_records(path, body))

    # §8.4: the media are counted, never listed, so what is there is compared with the count.
    media_root = os.path.join(root, "media")
    present = sum(len(names) for _, _, names in os.walk(media_root))
    if present != manifest.get("media_count"):
        problems.append(
            f"media/ holds {present} objects and the manifest says {manifest.get('media_count')}"
        )

    for problem in problems:
        print(f"verify-tenant-export: {problem}", file=sys.stderr)
    if problems:
        return 1
    print(
        f"the archive reads as tenant-export.md describes it: "
        f"{len(files)} data files, {sum(counts.values())} records, {present} media objects"
    )
    return 0


def read_records(path: str, body: bytes) -> list[str]:
    """§5 and §8.5: every line is one record, and none of them carries a workspace.

    The scope check has teeth by omission - "no record carries a tenant_id, so the archive has
    exactly one workspace by construction" - so a `tenant_id` appearing anywhere is a defect in the
    writer even when its value is the right one.
    """
    problems: list[str] = []
    for number, line in enumerate(body.splitlines(), start=1):
        if not line.strip():
            problems.append(f"{path}:{number} is blank; every line is one record (§5)")
            continue
        try:
            record = json.loads(line)
        except json.JSONDecodeError as broken:
            problems.append(f"{path}:{number} is not a record: {broken}")
            continue
        if record.get("op") != "UPSERT":
            problems.append(f"{path}:{number} has op {record.get('op')!r}; a FULL export has only UPSERT (§5)")
        for field in ("id", "updated_at", "data"):
            if field not in record:
                problems.append(f"{path}:{number} has no {field} (§5)")
        if "tenant_id" in (record.get("data") or {}):
            problems.append(f"{path}:{number} carries a tenant_id, which §8.5 forbids")
    return problems


if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("usage: verify-tenant-export.py <archive directory> <tenant uuid>", file=sys.stderr)
        raise SystemExit(2)
    raise SystemExit(main(sys.argv[1], sys.argv[2]))
