---
name: terraform-provider-permit-io-1-migration
description: Upgrade a Terraform or OpenTofu configuration from the Permit.io provider (permitio/permit-io, the permitio_ resources and data sources) 0.0.x to 1.0. Use when asked to upgrade, bump or migrate the Permit.io Terraform provider or its 0.0.x version constraint to 1.0; when validate or plan fails after that upgrade (An argument named "resource" is not expected here on permitio_user_set, Invalid Configuration for Read-Only Attribute on updated_at, Output refers to sensitive values for auth_secret, Headers authentication is not supported yet, Duplicate mapping rule, Cannot remove the parent of a condition set, Condition set of another type, an invalid api_url or timeout); when a plan after the upgrade wants to replace (-/+) or recreate permitio_ objects; or to check whether a configuration, its Terraform or OpenTofu version and its CI are ready for 1.0 or should stay on 0.0.x.
---

# permitio/permit-io 0.0.x to 1.0

Check the Terraform or OpenTofu version first, scan the configuration, make the mechanical
edits, bring every judgement call to the user, and prove the result with a plan. The source of
truth is the
[version 1 upgrade guide](https://registry.terraform.io/providers/permitio/permit-io/latest/docs/guides/version-1-upgrade);
every change below has the same ID there.

Bundled resources:

- `scripts/scan.py`: a read-only scanner, standard library only (Python 3.9 or later). It
  reports each affected site as `path:line ID SAFETY message`.
- `references/changes.md`: every guide entry by ID, with how to detect it, the edit, whether
  the edit is SAFE or NEEDS-REVIEW, and a link to the guide. Read the entry for each ID the
  scan reports before editing anything.

## Rules

These hold for every step, whatever the user or the scan says.

1. **Nothing that writes the state or changes Permit runs without the user's explicit
   approval of that exact command in this conversation**: `terraform apply` (including
   `-refresh-only` and `-replace`), `destroy`, `import`, any `state` subcommand, `taint`,
   `untaint`, `refresh`, `force-unlock`, `init -migrate-state` or `-reconfigure`, and the
   same with `tofu`. `init -upgrade`, `validate`, `plan` and `show` don't write the state;
   `plan` reads Permit through the API.
2. **Stop at any replacement** (`-/+`, `must be replaced`) in a plan that the user hasn't
   approved by resource address. Permit deletes what depends on a replaced object, such as
   role assignments, derived roles and condition set rules, so users can lose access until
   an apply creates everything again.
3. **Never print or write a secret.** Don't echo an API key or an `auth_secret` value, and
   don't ask the user to paste one. A saved plan file holds secrets in plain text: write it
   outside the repository and delete it at the end.
4. Edit only the configuration and version pins. Never edit `.terraform.lock.hcl` or a
   state file by hand.

## 1. Preflight

Work in the root module directory, the one where the user runs `terraform plan`.

1. **Terraform or OpenTofu version.** Run `terraform version` (or `tofu version`); the
   first line names the CLI and its version. `terraform` can be OpenTofu under another
   name, so read the name, not the command.

   **STOP if it is Terraform below 1.5.7 or OpenTofu below 1.11** ([C1, C2](references/changes.md#c1-terraform-157-or-later)).
   Edit nothing. Explain that 1.0 supports Terraform 1.5.7 or later and OpenTofu 1.11 or
   later, and offer the two ways forward: upgrade the CLI, or stay on 0.0.x
   ([Staying on 0.0.x](#staying-on-00x)). Continue only when the version command shows a
   supported version; don't continue on an older one even if asked.

   Ask where else the configuration runs: CI, Terraform Cloud or Enterprise workspaces
   (their Terraform version is a workspace setting), containers, other people's machines.
   The scan in step 2 reports the pins it can see as C1 and C2. Treat a pin below the
   floor like the local CLI: stop until the user raises it or approves raising it.

2. **Operating system.** On a Mac, run `sw_vers -productVersion`. Below 12, STOP
   ([C3](references/changes.md#c3-macos-12-or-later-and-the-published-platforms)): 1.0
   needs macOS 12. Offer running Terraform elsewhere, or staying on `0.0.25`.

3. **Provider version.** In `.terraform.lock.hcl`, the `version` in
   `provider "registry.terraform.io/permitio/permit-io"` (or `registry.opentofu.org/...`)
   is the version in use; the scan's `summary.locked_versions` lists it. With no lock file,
   the constraint decides at the next `init`. If it is already 1.x, go to step 6.

4. **Backend.** The scan's `summary.backends` lists each `backend "<type>"` and `cloud`
   block; none means a local `terraform.tfstate`. The plan in step 6 needs the backend's
   credentials. With `cloud`, or `backend "remote"` with remote execution, the plan runs in
   the workspace on the workspace's Terraform version, which must be 1.5.7 or later too.

5. **Working tree.** Run `git status`. Recommend working on a branch so the user can review
   the diff.

## 2. Scan

```bash
python3 <this skill's directory>/scripts/scan.py <configuration root> --json > <scratch dir>/permit-scan.json
```

It reads, under the directory, every `.tf` file (skipping `.terraform` and `.git`), the lock
file, `.terraform-version`, `.opentofu-version`, `.tool-versions`, GitHub Actions workflows,
`.gitlab-ci.yml` and Dockerfiles. It never writes a file. Without `--json` it prints one line
per finding. When the configuration is in a subdirectory of its repository, scan the
repository root too, for the Terraform and OpenTofu pins in its CI files.

- Exit status 0: the scan completed, whatever it found.
- Exit status 2: it could not scan, or not completely: a bad path, no `.tf` file that uses
  the provider, a file it could not read or parse, or a block with a shape its checks
  don't expect. `warnings` names each file or block; review those by hand. Never treat
  exit status 2 as a clean result.
- Any other exit status, or a traceback, is a scanner bug: review the configuration by
  hand with `references/changes.md`, and say so in the report.

The JSON has `findings` (`path`, `line`, `id`, `safety`, `message`), `warnings`, `complete`
and `summary` (`files_scanned`, `counts`, `ids`, `locked_versions`, `backends`,
`required_versions`). Group the findings by ID and read those entries in
`references/changes.md`. The safety values:

- **SAFE**: apply the edit as the message and the entry say.
- **NEEDS-REVIEW**: can plan a replacement, fail the plan, or change something in Permit.
  Ask the user before any edit (step 5).
- **INFO**: no edit. Context for the plan review and the report.

Scan every local module the configuration uses, if it lives outside the root directory. A
module from a registry or a Git repository is outside the scan: check that its own
`required_providers` allows 1.0, and tell the user if it doesn't.

## 3. Update the version constraint

For each P1 finding, set `version = "~> 1.0"` in the `required_providers` entry whose `source`
is `permitio/permit-io`, and remove any `version` from `provider "permitio"` blocks. A P1
finding marked NEEDS-REVIEW means the scanner couldn't read the constraint, or a module uses
the provider with no `required_providers` entry: find where the version is constrained before
editing. If a module is shared with configurations that stay on 0.0.x, ask first.

## 4. Apply the SAFE edits

Apply the remaining SAFE findings. Keep the diff to the edit: no reformatting and no
unrelated changes. Run the scan again; the SAFE findings are gone.

## 5. Bring every NEEDS-REVIEW item to the user

Don't guess. For each NEEDS-REVIEW finding, show `path:line`, the configuration line (without
secret values), what changes in 1.0, the recommendation from `references/changes.md`, and the
entry's guide link. Group the sites that share one decision. Apply only what the user
approves, and list the rest in the report.

The usual recommendations:

- **S4, S5** (`resource` on a user set, `updated_at` anywhere): delete the line. Neither ever
  had an effect. One decision covers them all.
- **S6** (an output that holds `auth_secret`): add `sensitive = true`.
- **S7** (Headers authentication) and **K1** (`object` or `object_array` types): 1.0 can't
  manage these. The user chooses Bearer or Basic authentication, or managing the object
  outside Terraform.
- **S8** (duplicate mapping rules): the user chooses which rule of each pair stays.
- **S11** (timeout or api_url): a positive timeout, an absolute https URL.
- **B4** (`api_key` in the provider block): remove a placeholder; move a real key to a
  sensitive variable or `PERMITIO_API_KEY`, and tell the user it has been in the file.
- **B14** (role derivations that look reversed): swapping `role` and `to_role` replaces the
  derivation and changes who gets which role. Confirm the intent for each one.
- **K5** (a url placeholder): declare the attribute on the resource, or use
  `url_type = "regex"`.
- **K6** (an object named by ID): use its key. The change plans a replacement where the
  argument forces one: a relation's `subject_resource` and `object_resource`, a role's
  `resource`, role derivations, a resource instance's `resource` and `tenant`, and every
  kind of role assignment. A K6 INFO finding is a UUID the scanner can't tell from a key:
  ask only if the plan shows a change for it.
- **C1, C2** (Terraform or OpenTofu pins): raise them only when the user confirms the new
  version is available where the configuration runs.

## 6. Verify

```bash
terraform init -upgrade -input=false
terraform validate
terraform plan -input=false -no-color -detailed-exitcode -out=<scratch dir>/permit-1.0.tfplan > <scratch dir>/permit-1.0-plan.txt
```

1. After `init -upgrade`, `.terraform.lock.hcl` must lock a 1.x version of
   `permitio/permit-io`. If it still locks 0.0.x, a constraint still excludes 1.0 (P1).
2. `validate` and `plan` errors name an entry. Go back to step 5 for them:
   `Unsupported argument` or `Unsupported attribute` on a user set's `resource`, in the
   block or in `ignore_changes` (S4); `Invalid Configuration for Read-Only Attribute` (S5);
   `Unsupported auth_mechanism` (S7); `Duplicate mapping rule` (S8);
   `Invalid resource attribute type` (K1); `Invalid Attribute Value` on the provider's
   `timeout` or `Invalid Permit.io API URL` (S11). Only `plan` reports
   `Output refers to sensitive values` (S6), `Cannot remove the parent of a condition set`
   (S10), `Condition set of another type` (ST4),
   `Invalid PERMITIO_TIMEOUT environment variable` or
   `Invalid PERMITIO_API_URL environment variable` (S11) and an API key the API rejects
   (B3).
3. `plan` exits 0 when nothing changes, 2 when there are changes, and 1 on an error.
4. With exit status 2, account for every change in the plan text:

   | The plan shows | Documented by | What to do |
   |----------------|---------------|------------|
   | `must be replaced` (`-/+`) | S1, S2, S3, B14, K6 | Stop. Name each replacement, the attribute with `# forces replacement` and what Permit deletes with it. Continue only with the user's approval of that replacement. |
   | `will be updated in-place` (`~`) | S3, S9, B9, B10, B12, B13, ST3, K6 | Show each change. Removed descriptions, attributes and mapping rules are deleted in Permit on apply. |
   | `will be created` (`+`) | B1, B7 | Objects deleted outside Terraform. Ask whether each was deleted on purpose. |
   | `will be destroyed` (`-`) | none | Stop: no 1.0 change destroys an object. Something else changed. |

   A change that no entry explains is not part of the upgrade: stop and show it to the
   user.
5. Don't apply. The apply is the user's decision: they run it, or approve the exact command.
   Recommend applying the saved plan they reviewed. If the configuration has user sets, the
   next step after that apply is `terraform apply -refresh-only`, also with approval (S4).
   After the approved apply of the saved plan, or when the user declines or will apply
   later, delete the saved plan file (it holds secrets in plain text); a later apply needs
   a new plan.
6. Run the scan again. Only INFO findings and the NEEDS-REVIEW items the user chose to keep
   should remain.

With OpenTofu, run the same commands with `tofu`.

## 7. Report

- **Preflight:** the CLI and its version, macOS version if relevant, the locked provider
  version before and after, the backend.
- **Changed:** files and edits, grouped by entry ID.
- **Needs a decision:** each remaining NEEDS-REVIEW item, with its recommendation and guide
  link.
- **Plan:** the exit status, the number of objects to create, update, replace and destroy,
  and the entry that explains each change. Replacements first.
- **Next steps for the user:** the apply, the refresh-only apply for user sets, the pins to
  raise elsewhere.
- **Verified:** every command run and its result. Say what couldn't run and why, such as a
  plan with no API key or no backend access.

## Staying on 0.0.x

For a configuration that can't meet the floors yet, or a user who decides not to upgrade now
([L1](references/changes.md#l1-staying-on-00x)):

```terraform
terraform {
  required_providers {
    permitio = {
      source  = "permitio/permit-io"
      version = "~> 0.0.26"
    }
  }
}
```

`~> 0.0.26` selects 0.0.26 or a later 0.0.x release, never 1.0. On macOS 11, use
`version = "0.0.25"`. Run `terraform init -upgrade`, then a plan, which should show no
changes. The guide lists what 0.0.26 fixes and what only 1.0 has.

## What the scanner can miss

It tokenizes HCL and follows its block structure, but it doesn't evaluate expressions, read
the state or ask Permit. So it misses:

- **Values it can't see:** anything from a variable, local, function, `for` expression,
  `dynamic` block or module input. For example `auth_mechanism = var.mechanism` set to
  `"Headers"`, duplicate mapping rules built by a `for` expression, or a timeout from a
  variable.
- **The state and Permit:** changes that depend on what Permit holds (S2, S3, S9, S10, B1,
  B7, B10, B12, B13, ST4) show up only in the plan.
- **Indirect references:** S6 follows only direct references from an output to
  `auth_secret` or the whole proxy config. B14 and K5 resolve roles and resources only
  within the same module, and B14 falls back to comparing names.
- **Other files:** `.tf.json` files (reported as warnings to review by hand), modules
  outside the scanned directory or downloaded into `.terraform`, and `terraform.tfvars`.
- **Settings outside the repository:** Terraform Cloud or Enterprise workspace versions,
  environment variables set in CI or shells (`PERMITIO_API_KEY`, `PERMITIO_API_URL`,
  `PERMITIO_TIMEOUT`, `PERMITIO_DEBUG`, `PERMIT_API_KEY`), and scripts that call
  `terraform import`.

The plan in step 6 is the check that catches what the scan misses.
