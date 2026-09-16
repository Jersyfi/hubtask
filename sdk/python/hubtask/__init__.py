# SPDX-License-Identifier: BUSL-1.1
# Copyright (c) 2026 Jérôme Bastian Winkel
"""The Python client for the Hubtask API, generated from api/openapi.yaml (P-03, ADR-0057).

    from hubtask import Client, ProblemError

    client = Client("https://hubtask.example/api/v1", token="hbt_pat_...")
    page = client.list_containers(query={"type": "COLLECTION"})
    try:
        entry = client.create_work_item({"type": "TASK", "title": "x"}, idempotency_key=str(uuid.uuid4()))
    except ProblemError as refused:
        print(refused.status, refused.problem["code"])

`client.py` and `types.py` are generated; this file and `pyproject.toml` are the hand-written
half, kept to what a package needs so that the extraction ADR-0057 proposes is a move.
"""

from .client import Client, ProblemError

__all__ = ["Client", "ProblemError"]
