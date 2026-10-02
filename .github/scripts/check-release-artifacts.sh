#!/usr/bin/env bash
# Checks the release files GoReleaser wrote to a dist directory before they are
# published: one zip for each of the 13 platforms and no other zip, and the
# checksums file listing exactly those zips and the registry manifest, matching
# the zips, and listing terraform-registry-manifest.json as the manifest, which
# GoReleaser uploads under that name when it publishes. With -s, the checksums file
# must have a good signature. Each zip must hold one provider binary for its
# platform whose Go build info names this module, built from a clean tree at the
# release version.
#
# usage: check-release-artifacts.sh [-s] [-t TAG] DIST_DIR
#   -s      require the checksums signature and verify it with gpg
#   -t TAG  the release tag: the files are named for TAG without its v, and every
#           binary's module version must be TAG. Without -t, for a snapshot, the
#           version comes from DIST_DIR/metadata.json and every binary must have
#           the same module version.
#
# Run it from the repository root, which holds terraform-registry-manifest.json.
# Exit codes: 0 the files are complete, 1 a file is missing, extra or wrong, and
# 2 the check could not run.
set -euo pipefail

readonly PROJECT=terraform-provider-permit-io
readonly MODULE=github.com/permitio/terraform-provider-permit-io
readonly PLATFORMS=(
  darwin_amd64 darwin_arm64
  freebsd_386 freebsd_amd64 freebsd_arm freebsd_arm64
  linux_386 linux_amd64 linux_arm linux_arm64
  windows_386 windows_amd64 windows_arm64
)

usage() {
  echo "usage: $0 [-s] [-t TAG] DIST_DIR" >&2
  exit 2
}

signed=false
tag=
while getopts st: option; do
  case $option in
  s) signed=true ;;
  t) tag=$OPTARG ;;
  *) usage ;;
  esac
done
shift $((OPTIND - 1))
(($# == 1)) || usage
dist=$1
if [[ ! -d $dist ]]; then
  echo "::error title=Release artifacts::$dist is not a directory."
  exit 2
fi

for tool in go jq sha256sum unzip; do
  if ! command -v "$tool" >/dev/null; then
    echo "::error title=Release artifacts::$tool is not installed."
    exit 2
  fi
done
if [[ ! -f terraform-registry-manifest.json ]]; then
  echo "::error title=Release artifacts::Run from the repository root."
  exit 2
fi

if [[ -n $tag ]]; then
  version=${tag#v}
else
  if ! version=$(jq -er .version "$dist/metadata.json"); then
    echo "::error title=Release artifacts::No version in $dist/metadata.json."
    exit 2
  fi
fi

failures=0
fail() {
  echo "::error title=Release artifacts::$*"
  failures=$((failures + 1))
}

sums=${PROJECT}_${version}_SHA256SUMS
manifest=${PROJECT}_${version}_manifest.json
expected_zips=()
for platform in "${PLATFORMS[@]}"; do
  expected_zips+=("${PROJECT}_${version}_${platform}.zip")
done

expected=$(printf '%s\n' "${expected_zips[@]}" | sort)
found=$(find "$dist" -maxdepth 1 -name '*.zip' -exec basename {} \; | sort)
missing=$(comm -23 <(echo "$expected") <(echo "$found") | grep . || true)
extra=$(comm -13 <(echo "$expected") <(echo "$found") | grep . || true)
[[ -z $missing ]] || fail "Missing zips: ${missing//$'\n'/, }"
[[ -z $extra ]] || fail "Unexpected zips: ${extra//$'\n'/, }"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

if [[ ! -f $dist/$sums ]]; then
  fail "Missing $sums."
else
  listed=$(awk '{print $2}' "$dist/$sums" | sort)
  want=$(printf '%s\n' "${expected_zips[@]}" "$manifest" | sort)
  if [[ $listed != "$want" ]]; then
    fail "$sums does not list exactly the ${#PLATFORMS[@]} zips and $manifest."
  fi
  grep '\.zip$' "$dist/$sums" >"$work/zips.sha256" || true
  if ! (cd "$dist" && sha256sum --check --strict --quiet "$work/zips.sha256"); then
    fail "A zip does not match $sums."
  fi
  manifest_sum=$(awk -v name="$manifest" '$2 == name {print $1}' "$dist/$sums")
  read -r repo_manifest_sum _ < <(sha256sum terraform-registry-manifest.json)
  if [[ $manifest_sum != "$repo_manifest_sum" ]]; then
    fail "$sums does not list terraform-registry-manifest.json as $manifest."
  fi
  if [[ $signed == true ]]; then
    if [[ ! -f $dist/$sums.sig ]]; then
      fail "Missing $sums.sig."
    elif ! gpg --verify "$dist/$sums.sig" "$dist/$sums"; then
      fail "$sums.sig is not a good signature of $sums."
    fi
  fi
fi

module_versions=
for platform in "${PLATFORMS[@]}"; do
  zip=$dist/${PROJECT}_${version}_${platform}.zip
  [[ -f $zip ]] || continue
  binary=${PROJECT}_v${version}
  [[ $platform != windows_* ]] || binary+=.exe
  binaries=$(unzip -Z1 "$zip" | grep -c "^${PROJECT}_v" || true)
  if [[ $binaries != 1 ]] || ! unzip -q "$zip" "$binary" -d "$work/$platform"; then
    fail "$(basename "$zip") does not hold exactly one binary, $binary."
    continue
  fi
  info=$(go version -m "$work/$platform/$binary")
  module_version=$(awk -v module="$MODULE" '$1 == "mod" && $2 == module {print $3}' <<<"$info")
  if ! grep -qxF $'\tpath\t'"$MODULE" <<<"$info" || [[ -z $module_version ]]; then
    fail "$platform: the binary's build info does not name $MODULE."
    continue
  fi
  module_versions+=$module_version$'\n'
  if [[ $module_version == *+dirty ]] ||
    ! grep -qxF $'\tbuild\tvcs.modified=false' <<<"$info"; then
    fail "$platform: the binary was built from a modified tree ($module_version)."
  fi
  if [[ -n $tag && $module_version != "$tag" ]]; then
    fail "$platform: the binary's module version is $module_version, want $tag."
  fi
  goos=${platform%_*}
  goarch=${platform#*_}
  if ! grep -qxF $'\tbuild\tGOOS='"$goos" <<<"$info" ||
    ! grep -qxF $'\tbuild\tGOARCH='"$goarch" <<<"$info"; then
    fail "$platform: the binary is not built for $goos/$goarch."
  fi
done
distinct=$(sort -u <<<"$module_versions" | grep . || true)
if [[ $distinct == *$'\n'* ]]; then
  fail "The binaries have different module versions: ${distinct//$'\n'/, }"
fi

if ((failures > 0)); then
  echo "$failures problems with the release files in $dist."
  exit 1
fi
signature=
[[ $signed == false ]] || signature=", its signature"
echo "The release files in $dist are complete: ${#PLATFORMS[@]} platform zips," \
  "$sums$signature and the manifest entry. Every binary is $MODULE $distinct," \
  "built from a clean tree."
