# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
"""The Python client for the Hubtask API, generated from api/openapi.yaml.

    from hubtask import Client, ProblemError

    client = Client("https://hubtask.example/api/v1", token="hbt_pat_...")
    page = client.list_containers(query={"type": "COLLECTION"})
    try:
        entry = client.create_work_item({"type": "TASK", "title": "x"}, idempotency_key=str(uuid.uuid4()))
    except ProblemError as refused:
        print(refused.status, refused.problem["code"])

`client.py` and `types.py` are generated; this file and `pyproject.toml` are the hand-written
half, kept to what a package needs so that moving it to a repository of its own is a move.
"""

from .client import Client, ProblemError

__all__ = ["Client", "ProblemError"]
