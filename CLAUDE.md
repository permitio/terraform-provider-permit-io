# Claude Code Project Context

## Project
Terraform provider for Permit.io - manages Permit.io resources (resources, roles, relations, condition sets, proxy configs, etc.) via Terraform.

## Build & Test
```bash
GOTOOLCHAIN=auto go build ./...                    # Build
GOTOOLCHAIN=auto go test -count=1 -skip '^TestAcc' ./...  # Unit and offline tests (mock Permit API, no key needed)
PERMITIO_API_KEY=<key> GOTOOLCHAIN=auto TF_ACC=1 go test ./internal/provider/ -run <TestName> -v -timeout 300s  # Acceptance tests
prek install && prek run --all-files               # Hooks, as the CI prek job runs them (prek >= 0.5.3)
GOTOOLCHAIN=auto go tool govulncheck ./...         # Vulnerability scan
```
- Tool versions: golangci-lint, actionlint, zizmor and shellcheck in `.pre-commit-config.yaml` (Dependabot updates the SHAs and `# frozen:` tags), tfplugindocs and govulncheck in go.mod's `tool` directive. release.yml still pins its own govulncheck until the release workflow moves to `go tool govulncheck`.
- The zizmor findings the workflows already had are ignored one by one: inline `# zizmor: ignore[...]` comments in test.yml, line entries in `.github/zizmor.yml` for release.yml. Remove an ignore when its fix lands.

## Offline tests
- Offline provider tests run Terraform against `internal/acctest/mockpermit`, a fake Permit API. Set `TF_ACC_TERRAFORM_PATH=/path/to/terraform` or put `terraform` on `PATH`; otherwise terraform-plugin-testing downloads the latest Terraform release.
- `mockpermit.New(t, mockpermit.Tenants)` serves only the listed route sets and points the provider at the fake through `PERMITIO_API_URL`/`PERMITIO_API_KEY` with `t.Setenv`, so these tests cannot use `t.Parallel`. A request to a route the fake does not serve fails the test; add the route to the mock's route table. So does a path with an empty or dot segment, which usually means an empty key or ID.
- CI pipes `go test -json` into `internal/acctest/checkgotest`, which fails the run when a test skips or no test runs. An offline test must not call `t.Skip`.
- Name every test that needs the real API `TestAcc*`: the Build job skips those by name and fails on any other test that skips, including a `resource.Test` without `TF_ACC`.

## Testing with real API
- Use `PERMITIO_API_KEY` env var (not `PERMIT_API_KEY`)
- Provider env var is `PERMITIO_API_KEY`
- Tests create real resources in the Permit.io environment - clean up after
- To test locally with terraform CLI, build the binary and use a `terraformrc` with `dev_overrides`

## Known patterns & pitfalls
- **Null vs empty map**: Terraform distinguishes between null (field omitted) and empty map (`= {}`). When an Optional field's API returns an empty map but the user didn't specify the field, the provider must return null to match the plan. See `newAttributesModelsFromSDKWithPlan` for the pattern.
- **Return after errors**: Always `return` after `response.Diagnostics.AddError()` in CRUD methods. If you don't, the code continues to set state with zero-value models, causing secondary "MISSING TYPE" panics because uninitialized `types.Set` fields have no element type info.
- **Keys vs IDs in relations**: The `permitio_relation` resource's `subject_resource`/`object_resource` fields only work reliably with resource **keys**, not UUIDs. The API accepts both but always returns keys, causing state inconsistency when UUIDs are used.
- **Resource-scoped roles**: Use action keys only (e.g. `"read"`), not `"resource:action"` format. The `resource` field must reference an existing resource key.

## Code structure
- `internal/provider/` - All resource implementations
- Each resource type has: `resource.go` (schema + CRUD), `client.go` (API calls), `model.go` (data models)
- `internal/provider/common/` - Shared utilities
- `internal/acctest/` - Test support: `mockpermit` (the fake Permit API) and `checkgotest` (the CI test result checker)
- SDK: `github.com/permitio/permit-golang` v1.2.8
