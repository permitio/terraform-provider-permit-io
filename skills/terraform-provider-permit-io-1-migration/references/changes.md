# permitio/permit-io 0.0.x to 1.0: change catalogue

One section for each entry of the
[version 1 upgrade guide](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade),
under the same ID. `scripts/scan.py` reports sites with these IDs. Each section says:

- **Detect**: how to find the change in a configuration, or that there is nothing to
  find there because it is behaviour, or depends on the state or on what Permit holds.
  Those show up only in the plan.
- **Edit**: the change to make, if any.
- **Safety**: **SAFE** (apply the edit as written) or **NEEDS-REVIEW** (show the site to
  the user with the recommendation, and apply only what they approve). Anything that can
  plan a replacement, fail the plan or change something in Permit is NEEDS-REVIEW.
- **Scanner**: what `scripts/scan.py` reports for it: SAFE, NEEDS-REVIEW, INFO (context,
  no edit), or nothing.

No entry allows running `terraform apply`, `terraform import`, `terraform state`,
`terraform taint` or `terraform apply -refresh-only` without the user's approval of that
command.

## P: Provider version

### P1. The version constraint selects 1.0

[Guide: P1](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#p1-the-version-constraint-selects-10)

- Detect: the `version` of each `required_providers` entry whose `source` is
  `permitio/permit-io` (also written `registry.terraform.io/permitio/permit-io` or
  `registry.opentofu.org/permitio/permit-io`), in every module; an entry with no
  `version`; a `version` inside a `provider "permitio"` block. The locked version is in
  `.terraform.lock.hcl`.
- Edit: `version = "~> 1.0"`, and remove any `version` from the provider block. Then
  `terraform init -upgrade` updates `.terraform.lock.hcl`; never edit the lock file by
  hand. If a module is shared with configurations that stay on 0.0.x, ask first.
- Safety: SAFE.
- Scanner: SAFE for a constraint that excludes 1.0, one that also allows 2.0, a missing
  one, and a `version` in the provider block. NEEDS-REVIEW when it can't read the
  constraint, or when a module has `permitio_` objects but no `required_providers` entry
  for the provider: find where the version is constrained before editing.

## C: Compatibility

### C1. Terraform 1.5.7 or later

[Guide: C1](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#c1-terraform-157-or-later)

- Detect: `terraform version` wherever the configuration runs (SKILL.md, step 1); a
  `required_version` that allows versions below 1.5.7; Terraform pins in
  `.terraform-version`, `.tool-versions`, CI workflows (`terraform_version:`) and
  `hashicorp/terraform:` images. A Terraform Cloud or Enterprise workspace sets its
  version in the workspace settings, which only the user can check.
- Edit: upgrade Terraform where it runs and raise the pins. Optionally set
  `required_version = ">= 1.5.7"`.
- Safety: NEEDS-REVIEW. It changes which Terraform versions can run the configuration.
  Where Terraform is below 1.5.7, stop (SKILL.md, step 1).
- Scanner: NEEDS-REVIEW for each pin and `required_version` below 1.5.7.

### C2. OpenTofu 1.11 or later

[Guide: C2](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#c2-opentofu-111-or-later)

- Detect: `tofu version` wherever the configuration runs; OpenTofu pins in
  `.opentofu-version`, `.tool-versions` (`opentofu`), CI workflows (`tofu_version:`) and
  `opentofu/opentofu:` images.
- Edit: upgrade OpenTofu where it runs and raise the pins. The source stays
  `permitio/permit-io`.
- Safety: NEEDS-REVIEW. Where OpenTofu is below 1.11, stop.
- Scanner: NEEDS-REVIEW for each pin below 1.11.

### C3. macOS 12 or later, and the published platforms

[Guide: C3](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#c3-macos-12-or-later-and-the-published-platforms)

- Detect: nothing in the configuration. Ask where Terraform runs; on a Mac,
  `sw_vers -productVersion`. A platform outside the guide's list has no build.
- Edit: none. On macOS 11 or earlier, upgrade macOS or run Terraform elsewhere, or stay
  on `0.0.25` ([L1](#l1-staying-on-00x)).
- Safety: NEEDS-REVIEW. Stop on macOS 11 or earlier.
- Scanner: not reported.

## S: Schema

### S1. Changing `key` replaces the object

[Guide: S1](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s1-changing-key-replaces-the-object)

- Detect: in the plan, `must be replaced` on a `permitio_resource`, `permitio_user_set`,
  `permitio_resource_set`, `permitio_proxy_config` or `permitio_user_attribute`, with
  `# forces replacement` on `key`. That includes a key change whose apply failed with
  0.0.x. In the configuration: a `key` that comes from an expression (a variable, a
  local, `each.value`), which can change without the line changing.
- Edit: none. Never change a key without the user's approval. When the user means the
  replacement, add `replace_triggered_by` to the dependents, or plan a second apply, as
  the guide shows.
- Safety: NEEDS-REVIEW.
- Scanner: INFO for a key that comes from an expression.

### S2. Changing a relation's `name` or `description` replaces it

[Guide: S2](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s2-changing-a-relations-name-or-description-replaces-it)

- Detect: nothing in the configuration alone. The plan shows a `permitio_relation` that
  `must be replaced`, with `# forces replacement` on `name` or `description`, including
  a change whose apply failed with 0.0.x.
- Edit: none. Either set the name and description back to what Permit has, or, when the
  user means the replacement, add `replace_triggered_by` to the role derivations that the
  relation links.
- Safety: NEEDS-REVIEW.
- Scanner: not reported.

### S3. Changing `permitio_resource_set.resource` to another resource replaces the set

[Guide: S3](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s3-changing-permitio_resource_setresource-to-another-resource-replaces-the-set)

- Detect: nothing in the configuration alone. The plan shows a `permitio_resource_set`
  that `must be replaced`, with `# forces replacement` on `resource`. A switch between the
  key and the ID of the same resource is an in-place update instead.
- Edit: none. When the user means the replacement, add `replace_triggered_by` to the
  condition set rules on the set, or plan a second apply.
- Safety: NEEDS-REVIEW.
- Scanner: not reported.

### S4. `permitio_user_set.resource` is removed

[Guide: S4](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s4-permitio_user_setresource-is-removed)

- Detect: a `resource` argument in a `permitio_user_set` block, or `resource` in its
  `lifecycle { ignore_changes = [...] }`. Also any user set in a state written by 0.0.x.
- Edit: delete the argument and the `ignore_changes` entry. The API dropped the value,
  so it never had an effect. After the upgrade, run `terraform apply -refresh-only` once,
  only with the user's approval, even if no user set had `resource`: until an apply
  writes the state, `terraform show -json` and `terraform state show` fail on those user
  sets with `unsupported attribute "resource"`.
- Safety: NEEDS-REVIEW. Recommend the deletion.
- Scanner: NEEDS-REVIEW for the argument and for the `ignore_changes` entry. INFO on the
  first user set of a module where none has the argument, for the refresh-only apply.

### S5. `updated_at` is read-only

[Guide: S5](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s5-updated_at-is-read-only)

- Detect: `updated_at` in any `permitio_*` resource or data source block.
- Edit: delete the line. Setting it failed after every apply with 0.0.x.
- Safety: NEEDS-REVIEW. Recommend the deletion.
- Scanner: NEEDS-REVIEW.

### S6. `permitio_proxy_config.auth_secret` is sensitive

[Guide: S6](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s6-permitio_proxy_configauth_secret-is-sensitive)

- Detect: an `output` without `sensitive = true` whose `value` references
  `permitio_proxy_config.<name>.auth_secret`, a value inside it, or the whole
  `permitio_proxy_config.<name>` object. A reference through a local, a module output or
  a `for` expression shows up only in the plan, as `Output refers to sensitive values`.
- Edit: add `sensitive = true` to the output. Recommend also taking the secret from a
  variable marked `sensitive = true` instead of writing it in the configuration.
- Safety: NEEDS-REVIEW.
- Scanner: NEEDS-REVIEW.

### S7. Headers proxy authentication is rejected

[Guide: S7](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s7-headers-proxy-authentication-is-rejected)

- Detect: `auth_mechanism = "Headers"` on a `permitio_proxy_config`.
- Edit: none that keeps Headers authentication. Either switch to `Bearer` or `Basic`
  with a matching `auth_secret`, which changes how the proxy authenticates to the
  backend, or manage the proxy config outside Terraform until the provider supports
  Headers ([K2](#k2-headers-proxy-authentication)).
- Safety: NEEDS-REVIEW.
- Scanner: NEEDS-REVIEW.

### S8. Mapping rules with the same `url` and `http_method` are rejected

[Guide: S8](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s8-mapping-rules-with-the-same-url-and-http_method-are-rejected)

- Detect: two `mapping_rules` entries of one `permitio_proxy_config` with the same
  literal `url` and `http_method`, compared exactly, as the provider does. Rules built
  from variables or `for` expressions show up only in the plan.
- Edit: keep one rule for each pair. The user chooses which `resource` and `action`
  stay; Permit already kept only one of them on update.
- Safety: NEEDS-REVIEW.
- Scanner: NEEDS-REVIEW, on the second rule of each pair.

### S9. Removing a condition set description clears it

[Guide: S9](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s9-removing-a-condition-set-description-clears-it)

- Detect: nothing certain in the configuration: a `permitio_user_set` or
  `permitio_resource_set` with no `description` is affected only if Permit holds one.
  The plan shows an in-place update that removes `description`.
- Edit: to keep the description, add it to the configuration with the value the plan
  shows being removed.
- Safety: NEEDS-REVIEW.
- Scanner: not reported.

### S10. Removing `parent_id` fails at plan time

[Guide: S10](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s10-removing-parent_id-fails-at-plan-time)

- Detect: nothing certain in the configuration. The plan fails with
  `Cannot remove the parent of a condition set` for a set that has no `parent_id` in the
  configuration and a parent in Permit.
- Edit: recommend adding `parent_id` back, which changes nothing in Permit. The other
  choices: detach the set outside Terraform, then remove `parent_id`; or replace the set
  with `terraform taint` and an apply, which deletes its condition set rules in Permit.
  Both run only with the user's approval.
- Safety: NEEDS-REVIEW.
- Scanner: not reported.

### S11. The provider checks `api_url` and `timeout`

[Guide: S11](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s11-the-provider-checks-api_url-and-timeout)

- Detect: in a `provider "permitio"` block, a literal `timeout` of 0 or less, or above
  9223372036, and a literal `api_url` that isn't an absolute `http` or `https` URL.
  `PERMITIO_TIMEOUT` and `PERMITIO_API_URL` get the same checks: ask about CI settings
  and shell profiles.
- Edit: a positive `timeout` (0 or less meant no limit; the default is 10 seconds, and
  it now covers retries, [B2](#b2-requests-are-retried-and-timeout-covers-the-retries));
  an absolute `https://` URL.
- Safety: NEEDS-REVIEW.
- Scanner: NEEDS-REVIEW.

### S12. Data sources require only their key

[Guide: S12](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s12-data-sources-require-only-their-key)

- Detect: `name` or `actions` on `data "permitio_resource"`, `name` on
  `data "permitio_role"`, and `name`, `type` or `conditions` on
  `data "permitio_condition_set"`.
- Edit: optionally remove them. Configurations that keep them still work.
- Safety: SAFE.
- Scanner: INFO.

### S13. `permitio_user_attribute.description` is optional

[Guide: S13](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s13-permitio_user_attributedescription-is-optional)

- Detect: nothing to change.
- Edit: none.
- Safety: SAFE.
- Scanner: not reported.

### S14. Mapping rules can match a regular expression

[Guide: S14](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#s14-mapping-rules-can-match-a-regular-expression)

- Detect: nothing to change. `url_type` is a new optional argument.
- Edit: none. [K5](#k5-a-url-placeholder-in-a-mapping-rule-adds-a-resource-attribute)
  uses it.
- Safety: SAFE.
- Scanner: not reported.

## ST: State and import

### ST1. Import works on every resource

[Guide: ST1](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#st1-import-works-on-every-resource)

- Detect: nothing to change.
- Edit: none. Importing is optional and writes the state: only with the user's
  approval, after a plan with the `import` block.
- Safety: SAFE.
- Scanner: not reported.

### ST2. Composite import IDs are checked

[Guide: ST2](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#st2-composite-import-ids-are-checked)

- Detect: a literal `id` in an `import` block whose `to` is a `permitio_relation`,
  `permitio_role_derivation`, `permitio_role_assignment`,
  `permitio_resource_instance_role_assignment` or
  `permitio_group_resource_instance_role_assignment`, with the wrong number of
  `:`-separated parts or an empty part. Also scripts that run `terraform import` with
  such IDs.
- Edit: write the ID in the format of the guide's import table. Only the user knows
  which object an ID means.
- Safety: NEEDS-REVIEW.
- Scanner: NEEDS-REVIEW.

### ST3. One in-place update after importing with empty `attributes`

[Guide: ST3](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#st3-one-in-place-update-after-importing-with-empty-attributes)

- Detect: `attributes = {}` on a `permitio_resource`, and `attributes = jsonencode({})`
  on a `permitio_tenant` or `permitio_resource_instance`. It matters only after an
  import.
- Edit: none. The update it plans once changes nothing in Permit; leaving `attributes`
  out avoids it.
- Safety: NEEDS-REVIEW.
- Scanner: INFO.

### ST4. A condition set of the other type is refused

[Guide: ST4](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#st4-a-condition-set-of-the-other-type-is-refused)

- Detect: nothing in the configuration. The plan fails with
  `Condition set of another type`.
- Edit: move the set to the right resource type in the state: a `removed` block with
  `destroy = false` and an `import` block (Terraform 1.7 or later, or OpenTofu), or
  `terraform state rm` and `terraform import`. Both write the state: only with the
  user's approval.
- Safety: NEEDS-REVIEW.
- Scanner: not reported.

## B: Plan and wire behaviour

### B1. Objects deleted outside Terraform are created again

[Guide: B1](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b1-objects-deleted-outside-terraform-are-created-again)

- Detect: nothing in the configuration. The plan shows `will be created` for objects
  that are in the state.
- Edit: none. Ask whether each was deleted on purpose; if so, the user removes it from
  the configuration too, or the apply creates it again.
- Safety: NEEDS-REVIEW.
- Scanner: not reported.

### B2. Requests are retried, and `timeout` covers the retries

[Guide: B2](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b2-requests-are-retried-and-timeout-covers-the-retries)

- Detect: nothing to change.
- Edit: none. Raise `timeout` if 429 errors persist.
- Safety: SAFE.
- Scanner: not reported.

### B3. The provider checks the API key on every run

[Guide: B3](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b3-the-provider-checks-the-api-key-on-every-run)

- Detect: nothing in the configuration. Pipelines that plan with no valid key or no
  network access to the API, such as a plan-only job with a dummy key.
  `terraform validate` needs no key.
- Edit: give every plan and apply a valid environment-level API key, from the
  environment or a sensitive variable. Never write a key into a file or print it.
- Safety: NEEDS-REVIEW.
- Scanner: not reported. [B4](#b4-the-provider-block-overrides-environment-variables)
  covers keys written in the provider block.

### B4. The provider block overrides environment variables

[Guide: B4](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b4-the-provider-block-overrides-environment-variables)

- Detect: a literal `api_key` in a `provider "permitio"` block, whether a placeholder
  such as `YOUR_API_KEY` or a real key; a literal `api_url`, which now wins over
  `PERMITIO_API_URL`. The scanner never prints the value.
- Edit: remove a placeholder `api_key`. Move a real key to a sensitive variable or
  `PERMITIO_API_KEY`, and tell the user it has been in the file, so they can decide
  whether to rotate it. Remove `api_url` when runs set `PERMITIO_API_URL` to another URL.
- Safety: NEEDS-REVIEW.
- Scanner: NEEDS-REVIEW for a literal `api_key`; INFO for a literal `api_url`.

### B5. `PERMITIO_DEBUG` is no longer read

[Guide: B5](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b5-permitio_debug-is-no-longer-read)

- Detect: nothing in the configuration; `PERMITIO_DEBUG` in CI or shell settings.
- Edit: none needed. Use `TF_LOG=DEBUG` for the provider's debug logs.
- Safety: SAFE.
- Scanner: not reported.

### B6. Errors show the HTTP status and the API's message

[Guide: B6](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b6-errors-show-the-http-status-and-the-apis-message)

- Detect: nothing in the configuration; scripts that match the provider's error text,
  such as `ContextError`.
- Edit: update those scripts.
- Safety: SAFE.
- Scanner: not reported.

### B7. Role assignment reads find every assignment

[Guide: B7](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b7-role-assignment-reads-find-every-assignment)

- Detect: nothing in the configuration. The plan can show `will be created` for a
  tenant-level `permitio_role_assignment` deleted outside Terraform, and stops showing
  the create that repeated on every run for assignments past the first page.
- Edit: none.
- Safety: NEEDS-REVIEW.
- Scanner: not reported.

### B8. The group role assignment uses the provider's configuration

[Guide: B8](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b8-the-group-role-assignment-uses-the-providers-configuration)

- Detect: nothing in the configuration. Nothing reads `PERMIT_API_KEY` any more.
- Edit: none.
- Safety: SAFE.
- Scanner: not reported.

### B9. JSON arguments compare as JSON

[Guide: B9](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b9-json-arguments-compare-as-json)

- Detect: `attributes` on a `permitio_tenant` or `permitio_resource_instance`, or
  `conditions` on a `permitio_user_set` or `permitio_resource_set`, written as a heredoc
  or a JSON string rather than with `jsonencode`.
- Edit: none needed. An unchanged configuration that 0.0.x applied without errors plans
  nothing; a string reformatted since plans one in-place update that sends the same
  object. Rewriting it with `jsonencode` is optional and plans that one update too.
- Safety: NEEDS-REVIEW.
- Scanner: INFO.

### B10. Removing tenant or resource instance `attributes` clears them

[Guide: B10](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b10-removing-tenant-or-resource-instance-attributes-clears-them)

- Detect: nothing certain in the configuration. The plan shows an in-place update that
  removes `attributes` from a `permitio_tenant` or `permitio_resource_instance` whose
  configuration has none while Permit holds some.
- Edit: to keep them, add `attributes = jsonencode({...})` with the values the plan
  shows being removed.
- Safety: NEEDS-REVIEW.
- Scanner: not reported.

### B11. Descriptions left out of the configuration are kept

[Guide: B11](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b11-descriptions-left-out-of-the-configuration-are-kept)

- Detect: nothing to change.
- Edit: none.
- Safety: SAFE.
- Scanner: not reported.

### B12. Removing a resource's `attributes` clears them

[Guide: B12](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b12-removing-a-resources-attributes-clears-them)

- Detect: nothing certain in the configuration. The plan shows an in-place update of a
  `permitio_resource` that removes attributes Permit holds: ones the configuration
  doesn't declare, added outside Terraform or by a proxy mapping rule
  ([K5](#k5-a-url-placeholder-in-a-mapping-rule-adds-a-resource-attribute)). Permit also
  deletes the resource sets whose conditions reference a removed attribute.
- Edit: declare every attribute to keep, with the type Permit has for it.
- Safety: NEEDS-REVIEW.
- Scanner: not reported. K5 reports the mapping rules that add attributes.

### B13. Proxy mapping rules match the configuration after every apply

[Guide: B13](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b13-proxy-mapping-rules-match-the-configuration-after-every-apply)

- Detect: nothing certain in the configuration. The plan shows an in-place update of a
  `permitio_proxy_config` when Permit holds rules the configuration doesn't, such as
  rules added outside Terraform.
- Edit: add every rule to keep to the configuration.
- Safety: NEEDS-REVIEW.
- Scanner: not reported.

### B14. Role derivation `role` and `to_role`

[Guide: B14](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#b14-role-derivation-role-and-to_role)

- Detect: every `permitio_role_derivation`. `role` must be a role on `on_resource`, and
  `to_role` a role on `resource`. When both roles are `permitio_role` resources in the
  same module, their `resource` settles it; otherwise compare the role keys with the
  resource keys, as in the old example (`role = "fileAdmin"` with `resource = "file"`).
- Edit: swap `role` and `to_role` where they are reversed. Every argument of a role
  derivation forces a replacement, so the swap replaces the derivation, and it changes
  who gets which role.
- Safety: NEEDS-REVIEW.
- Scanner: NEEDS-REVIEW where they look reversed; INFO where the scanner can't tell.

## D: Deprecations

### D. Deprecations

[Guide: D](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#d-deprecations)

- Detect: nothing. 1.0 deprecates nothing for removal in 2.0;
  `permitio_user_set.resource` is removed rather than deprecated
  ([S4](#s4-permitio_user_setresource-is-removed)).
- Edit: none.
- Safety: SAFE.
- Scanner: not reported.

## K: Known issues

### K1. `object` and `object_array` attribute types

[Guide: K1](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#k1-object-and-object_array-attribute-types)

- Detect: `type = "object"` or `type = "object_array"` in a `permitio_resource`'s
  `attributes`, or on a `permitio_user_attribute`. An object in Permit with such an
  attribute fails to refresh, whether a resource or a data source reads it; that shows
  up only in the plan.
- Edit: none in the configuration. Manage these objects outside Terraform until the
  provider supports the types. Taking them out of Terraform uses a `removed` block with
  `destroy = false`, or `terraform state rm`: only with the user's approval.
- Safety: NEEDS-REVIEW.
- Scanner: NEEDS-REVIEW.

### K2. Headers proxy authentication

[Guide: K2](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#k2-headers-proxy-authentication)

- Detect: as [S7](#s7-headers-proxy-authentication-is-rejected).
- Edit: as S7.
- Safety: NEEDS-REVIEW.
- Scanner: not reported under K2; the scanner reports S7.

### K3. Clearing some descriptions

[Guide: K3](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#k3-clearing-some-descriptions)

- Detect: nothing certain in the configuration. Removing the `description` of a
  `permitio_tenant`, or of an action or attribute of a `permitio_resource`, doesn't clear
  it in Permit; for an action or attribute, the apply can fail with an inconsistent
  result.
- Edit: keep the description in the configuration, or clear it in the Permit UI or with
  the API first and then remove it.
- Safety: NEEDS-REVIEW.
- Scanner: not reported.

### K4. Removing a parent from a condition set

[Guide: K4](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#k4-removing-a-parent-from-a-condition-set)

- Detect: as [S10](#s10-removing-parent_id-fails-at-plan-time).
- Edit: as S10.
- Safety: NEEDS-REVIEW.
- Scanner: not reported.

### K5. A URL placeholder in a mapping rule adds a resource attribute

[Guide: K5](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#k5-a-url-placeholder-in-a-mapping-rule-adds-a-resource-attribute)

- Detect: a mapping rule `url` with a placeholder such as `{invoice_id}`, without
  `url_type = "regex"`. Permit adds an attribute of that name to the rule's `resource`.
  When a `permitio_resource` in the same module manages that resource, by reference or
  key, and doesn't declare the attribute, its plans remove it
  ([B12](#b12-removing-a-resources-attributes-clears-them)).
- Edit: declare the attribute in the resource's `attributes` with the type Permit gives
  it, or rewrite the url as a regular expression with `url_type = "regex"`.
- Safety: NEEDS-REVIEW.
- Scanner: NEEDS-REVIEW when a managed resource doesn't declare the attribute; INFO when
  the scanner can't find the resource in the module.

### K6. Name other objects by key

[Guide: K6](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#k6-name-other-objects-by-key)

- Detect: an argument that names another object with an `.id` reference, such as
  `permitio_resource.document.id`, or a UUID: a relation's `subject_resource` and
  `object_resource`, a resource set's `resource`, a role's `resource`, `extends` and
  `permissions`, every argument of a role derivation, and the user, group, role,
  resource, instance and tenant arguments of resource instances and role assignments.
  The plan shows a change on every run, or the apply fails with an inconsistent result.
  A UUID can also be the object's key, as when user keys come from an identity
  provider: it names the object by key when a `permitio_` object has it as its `key`.
  Condition set rules keep their arguments as written, so an ID there doesn't drift.
- Edit: use the key (`.key`). The next plan can show an in-place update (a role's
  `extends` and `permissions`, or a resource set's `resource` changed from the ID to the
  key of the same resource), or a replacement where the argument forces one: a
  relation's `subject_resource` and `object_resource`, a role's `resource`, every
  argument of a role derivation, a resource instance's `resource` and `tenant`, and the
  arguments of role assignments, resource instance role assignments and group resource
  instance role assignments.
- Safety: NEEDS-REVIEW.
- Scanner: NEEDS-REVIEW for an `.id` reference. INFO for a UUID that no `permitio_`
  object in the module has as its key: the scanner can't tell an ID from a key there.

## KP: Kept on purpose

### KP1. Role `permissions` and `extends`

[Guide: KP1](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#kp1-role-permissions-and-extends)

- Detect: nothing to change; this works as in 0.0.x. A `permitio_role` without
  `permissions` or `extends` keeps what the role has in Permit.
- Edit: none. Setting either to `[]` removes them all in Permit: only when the user asks.
- Safety: NEEDS-REVIEW.
- Scanner: not reported.

### KP2. `mapping_rules` stays a list

[Guide: KP2](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#kp2-mapping_rules-stays-a-list)

- Detect: nothing to change. Reordering the list plans an in-place update that reorders
  the rules in Permit.
- Edit: none.
- Safety: NEEDS-REVIEW.
- Scanner: not reported.

### KP3. The provider name `permitio` and the registry name `permit-io`

[Guide: KP3](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#kp3-the-provider-name-permitio-and-the-registry-name-permit-io)

- Detect: a `required_providers` entry for `permitio/permit-io` under a local name other
  than `permitio`. Every `permitio_` object then needs `provider = <that name>`.
- Edit: none needed. Renaming the local name to `permitio` means changing every
  `provider "<name>"` block and `provider = <name>` argument with it: only when the user
  asks.
- Safety: NEEDS-REVIEW.
- Scanner: INFO.

## L: Staying on 0.0.x

### L1. Staying on 0.0.x

[Guide: L1](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade#l1-staying-on-00x)

- Detect: nothing to find. This is the alternative to upgrading, when Terraform,
  OpenTofu, a CI pin or macOS is below the floors (C1 to C3) and can't move yet, or the
  user decides not to upgrade now.
- Edit: `version = "~> 0.0.26"`, which selects 0.0.26 or a later 0.0.x release and never
  1.0; on macOS 11, `version = "0.0.25"`. Then `terraform init -upgrade`.
- Safety: NEEDS-REVIEW. It is the user's decision.
- Scanner: not reported.
