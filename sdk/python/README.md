# The Python SDK

`hubtask` is the Python client for the Hubtask API, generated from
[`api/openapi.yaml`](../../api/openapi.yaml) by `make generate` through `tools/sdkgen`. One
method per operation — `create_work_item`, `list_containers`, `sync_pull` — with the path
parameters positional, the body as a `TypedDict` from `hubtask.types`, and the query and the
headers as keyword arguments. No dependency: `urllib` and the standard library, Python 3.11 or
newer.

```python
import uuid
from hubtask import Client, ProblemError

client = Client("https://hubtask.example/api/v1", token="hbt_pat_…")

for collection in client.list_containers(query={"type": "COLLECTION"})["data"]:
    print(collection["id"], collection["name"])

try:
    entry = client.create_work_item(
        {"collection_id": "…", "type": "TASK", "title": "Made by the Python SDK"},
        idempotency_key=str(uuid.uuid4()),
    )
except ProblemError as refused:
    print(refused.status, refused.problem["code"], refused.problem.get("field_errors"))
```

[`examples/quickstart.py`](./examples/quickstart.py) lists a hub's collections, creates an entry
and reads it back; `python3 sdk/python/examples/quickstart.py` with `HUBTASK_URL`,
`HUBTASK_TOKEN` and `HUBTASK_HUB` set.

What is hand-written is `hubtask/__init__.py`, `pyproject.toml` and this file; `client.py` and
`types.py` are regenerated from the contract on every `make generate`, and `test/contract`
drives the example against the in-process server where a `python3` is on the path.

**Licence.** Apache-2.0 — the `LICENSE` file beside this README, the header every file carries,
and the classifier in `pyproject.toml` ([ADR-0059](../../docs/adr/ADR-0059-licensing-phases-and-licensing-start.md)
§6, deciding [ADR-0057](../../docs/adr/ADR-0057-sdk-licence-and-extraction.md)). The package name
on PyPI and an extraction into a repository of its own stay open.
