# Claude Code Project Context

## Project
Terraform provider for Permit.io - manages Permit.io resources (resources, roles, relations, condition sets, proxy configs, etc.) via Terraform.

## Build & Test
```bash
GOTOOLCHAIN=auto go build ./...                    # Build
PERMITIO_API_KEY=<key> GOTOOLCHAIN=auto TF_ACC=1 go test ./internal/provider/ -run <TestName> -v -timeout 300s  # Acceptance tests
```

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
- SDK: `github.com/permitio/permit-golang` v1.2.8
