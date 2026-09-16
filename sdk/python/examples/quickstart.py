# SPDX-License-Identifier: BUSL-1.1
# Copyright (c) 2026 Jérôme Bastian Winkel
"""The Python client's example: list the collections of a hub, create an entry in the first one,
read it back.

    HUBTASK_URL=https://hubtask.example/api/v1 HUBTASK_TOKEN=hbt_pat_... HUBTASK_HUB=<hub-id> \\
      python3 sdk/python/examples/quickstart.py
"""

from __future__ import annotations

import os
import sys
import uuid

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from hubtask import Client, ProblemError  # noqa: E402


def run(base_url: str, token: str, hub: str, log=print) -> dict:
    client = Client(base_url, token=token)
    collections = client.list_containers(query={"type": "COLLECTION", "parent_id": hub})
    if not collections["data"]:
        raise SystemExit("the hub has no collection to create in")
    for collection in collections["data"]:
        log(f"{collection['id']}  {collection['name']}")

    # The idempotency key is what makes running this twice after a lost connection safe.
    first = collections["data"][0]["id"]
    created = client.create_work_item(
        {"collection_id": first, "type": "TASK", "title": "Made by the Python SDK"},
        idempotency_key=str(uuid.uuid4()),
    )
    log(f"created {created['id']} (version {created['version']})")
    read = client.get_work_item(created["id"])
    log(f"read {read['title']!r}")
    return read


if __name__ == "__main__":
    base_url, token, hub = os.environ.get("HUBTASK_URL"), os.environ.get("HUBTASK_TOKEN"), os.environ.get("HUBTASK_HUB")
    if not (base_url and token and hub):
        print("set HUBTASK_URL, HUBTASK_TOKEN and HUBTASK_HUB", file=sys.stderr)
        sys.exit(1)
    try:
        run(base_url, token, hub)
    except ProblemError as refused:
        print(f"refused: {refused} (request {refused.problem.get('request_id', '')})", file=sys.stderr)
        sys.exit(1)
