# The Go SDK

`sdk/go/hubtask` is the Go client for the Hubtask API, generated from
[`api/openapi.yaml`](../../api/openapi.yaml) by `make generate` (through
[`oapi-codegen.yaml`](./oapi-codegen.yaml) beside this file). One method per operation, typed
parameters and bodies, and a `…WithResponse` variant that decodes the answer into the schema the
contract declares for each status.

```go
import "github.com/Jersyfi/hubtask/sdk/go/hubtask"

client, err := hubtask.NewClientWithResponses("https://hubtask.example/api/v1",
    hubtask.WithRequestEditorFn(hubtask.WithBearer(token)))

res, err := client.ListContainersWithResponse(ctx, &hubtask.ListContainersParams{})
if err := hubtask.Check(err, res); err != nil {
    var problem *hubtask.ProblemError
    if errors.As(err, &problem) { /* problem.Problem.Code, problem.Problem.FieldErrors */ }
}
for _, container := range res.JSON200.Data { … }
```

[`examples/quickstart`](./examples/quickstart/main.go) lists a hub's collections, creates an
entry and reads it back; `go run ./sdk/go/examples/quickstart` with `HUBTASK_URL`,
`HUBTASK_TOKEN` and `HUBTASK_HUB` set.

What is hand-written is [`hubtask.go`](./hubtask/hubtask.go) and nothing else: the bearer, the
idempotency and `If-Match` editors, and `Check`. Everything else is regenerated from the contract
on every `make generate`, and `test/contract` drives the client against the in-process server so
that the two halves cannot drift apart.

**Licence.** The package is under the repository's licence (BSL 1.1) until
[ADR-0057](../../docs/adr/ADR-0057-sdk-licence-and-extraction.md) is accepted, which puts the
SDKs' own licence and their extraction to the owner.
