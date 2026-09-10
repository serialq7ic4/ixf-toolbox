#!/bin/sh
set -eu

release_version="3.26.3"
repository="serialq7ic4/ixf-toolbox"
apply=false
dry_run=true
seen_apply=false
seen_dry_run=false
install_dir=""

fail() {
    printf 'ERROR %s\n' "$*" >&2
    exit 1
}

json_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --dry-run)
            seen_dry_run=true
            dry_run=true
            shift
            ;;
        --apply)
            seen_apply=true
            apply=true
            dry_run=false
            shift
            ;;
        --install-dir)
            [ "$#" -ge 2 ] || fail "--install-dir requires a value"
            install_dir=$2
            shift 2
            ;;
        --help|-h)
            printf '%s\n' 'usage: bootstrap-runtime.sh [--dry-run|--apply] [--install-dir DIR]'
            exit 0
            ;;
        *)
            fail "unsupported argument: $1"
            ;;
    esac
done

if [ "$seen_apply" = true ] && [ "$seen_dry_run" = true ]; then
    fail "--dry-run and --apply are mutually exclusive"
fi

[ -n "${HOME:-}" ] || fail "HOME is required"
[ -d "$HOME" ] || fail "HOME does not exist"
if [ -z "$install_dir" ]; then
    install_dir="$HOME/.local/share/ixf-toolbox/bin"
else
    case "$install_dir" in
        '~') install_dir=$HOME ;;
        '~/'*) install_dir="$HOME/${install_dir#\~/}" ;;
        /*) ;;
        *) install_dir="$PWD/$install_dir" ;;
    esac
fi
while [ "$install_dir" != "/" ] && [ "${install_dir%/}" != "$install_dir" ]; do
    install_dir=${install_dir%/}
done
case "$install_dir" in
    *'/../'*|*'/..'|*'/./'*|*'/.'|'.'|'..')
        fail "install directory must be inside the current user directory and cannot contain traversal"
        ;;
esac

home_real=$(CDPATH= cd -- "$HOME" 2>/dev/null && pwd -P) || fail "cannot resolve HOME"
probe=$install_dir
suffix=""
while [ ! -e "$probe" ]; do
    name=${probe##*/}
    [ -n "$name" ] || fail "cannot resolve install directory"
    if [ -n "$suffix" ]; then
        suffix="$name/$suffix"
    else
        suffix=$name
    fi
    parent=${probe%/*}
    [ -n "$parent" ] || parent=/
    [ "$parent" != "$probe" ] || fail "cannot resolve install directory"
    probe=$parent
done
[ -d "$probe" ] || fail "install directory ancestor is not a directory"
probe_real=$(CDPATH= cd -- "$probe" 2>/dev/null && pwd -P) || fail "cannot resolve install directory"
install_real=$probe_real
if [ -n "$suffix" ]; then
    install_real="$probe_real/$suffix"
fi
case "$install_real" in
    "$home_real"|"$home_real"/*) ;;
    *) fail "install directory must resolve inside the current user directory" ;;
esac

command -v curl >/dev/null 2>&1 || fail "missing required tool: curl"
if command -v sha256sum >/dev/null 2>&1; then
    checksum_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
    checksum_tool=shasum
else
    fail "missing required tool: sha256sum or shasum"
fi

case "$(uname -s)" in
    Darwin) target_os=darwin ;;
    Linux) target_os=linux ;;
    *) fail "unsupported operating system: $(uname -s)" ;;
esac
case "$(uname -m)" in
    x86_64|amd64) target_arch=amd64 ;;
    arm64|aarch64) target_arch=arm64 ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
esac

asset="ixf_${release_version}_${target_os}_${target_arch}"
checksum_asset="ixf_${release_version}_checksums.txt"
release_base="https://github.com/${repository}/releases/download/v${release_version}"
if [ -n "${IXF_BOOTSTRAP_RELEASE_BASE_URL:-}" ]; then
    if [ "${IXF_BOOTSTRAP_TESTING:-}" != "1" ]; then
        case "$IXF_BOOTSTRAP_RELEASE_BASE_URL" in
            https://*) fail "release URL override is test-only" ;;
            *) fail "release URL override must use HTTPS unless IXF_BOOTSTRAP_TESTING=1" ;;
        esac
    fi
    release_base=${IXF_BOOTSTRAP_RELEASE_BASE_URL%/}
fi
release_host=${release_base#*://}
release_host=${release_host%%/*}
[ -n "$release_host" ] || fail "release host is empty"
target_path="$install_dir/ixf"

emit_result() {
    printf '{"ok":true,"dryRun":%s,"apply":%s,"version":"%s","asset":"%s","releaseHost":"%s","targetPath":"%s"}\n' \
        "$dry_run" "$apply" \
        "$(json_escape "$release_version")" \
        "$(json_escape "$asset")" \
        "$(json_escape "$release_host")" \
        "$(json_escape "$target_path")"
}

if [ "$apply" != true ]; then
    emit_result
    exit 0
fi

temp_dir=""
target_temp=""
cleanup() {
    [ -z "$target_temp" ] || rm -f -- "$target_temp"
    [ -z "$temp_dir" ] || rm -rf -- "$temp_dir"
}
trap cleanup EXIT HUP INT TERM
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/ixf-toolbox-bootstrap.XXXXXX") || fail "cannot create temporary directory"
downloaded_asset="$temp_dir/$asset"
downloaded_checksums="$temp_dir/$checksum_asset"
curl --fail --silent --show-error --location --output "$downloaded_asset" "$release_base/$asset" || fail "download failed: $asset"
curl --fail --silent --show-error --location --output "$downloaded_checksums" "$release_base/$checksum_asset" || fail "download failed: $checksum_asset"

expected_checksum=$(awk -v name="$asset" '
    {
        hash=$1
        file=$2
        sub(/^\*/, "", file)
        if (file == name && length(hash) == 64 && hash !~ /[^0-9A-Fa-f]/) {
            print tolower(hash)
            count++
        }
    }
    END { if (count != 1) exit 1 }
' "$downloaded_checksums") || fail "checksum file does not contain exactly one entry for $asset"
if [ "$checksum_tool" = sha256sum ]; then
    checksum_output=$(sha256sum "$downloaded_asset") || fail "cannot hash downloaded asset"
else
    checksum_output=$(shasum -a 256 "$downloaded_asset") || fail "cannot hash downloaded asset"
fi
actual_checksum=${checksum_output%% *}
[ "$actual_checksum" = "$expected_checksum" ] || fail "checksum mismatch for $asset"

mkdir -p -- "$install_dir" || fail "cannot create install directory"
target_temp="$install_dir/.ixf.$$.tmp"
rm -f -- "$target_temp"
cp -- "$downloaded_asset" "$target_temp" || fail "cannot stage installed binary"
chmod 0755 "$target_temp" || fail "cannot mark installed binary executable"
mv -f -- "$target_temp" "$target_path" || fail "cannot replace installed binary"
target_temp=""
emit_result
