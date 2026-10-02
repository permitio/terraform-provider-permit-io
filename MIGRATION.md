# Migrating between major versions

## 0.0.x to 1.0

Follow the [version 1 upgrade guide](docs/guides/version-1-upgrade.md), also on the
[Terraform Registry](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade).
It lists every change in 1.0 with who is affected, what to do, and the plan impact,
and says how to stay on 0.0.x.

The guide is generated from `templates/guides/version-1-upgrade.md.tmpl`: edit the
template, then run `go generate ./...`.
