#!/usr/bin/env bash
# Test helper: assembles dnf repodata in OUT_DIR from the "rpm" entries of
# one or more repo-meta.json files, as the website Worker will (LR-12): each
# file's elements in order, between the headers createrepo_c writes, then a
# repomd.xml naming the three gzipped files. Packages are not copied; only
# metadata queries (dnf repoquery) work on the result.
#
#   assemble-rpm-repo.sh OUT_DIR REPO_META_JSON...
set -euo pipefail
export LC_ALL=C

if [ "$#" -lt 2 ]; then
    printf 'Usage: %s OUT_DIR REPO_META_JSON...\n' "$0" >&2
    exit 2
fi
out=$1
shift

mkdir -p -- "$out/repodata"
count=$(jq -s '[.[].rpm | length] | add' "$@")
timestamp=${SOURCE_DATE_EPOCH:-$(date +%s)}

# write NAME FIELD OPEN CLOSE META...: repodata/NAME.xml.gz from every
# entry's FIELD.
write() {
    local name=$1 field=$2 open=$3 close=$4
    shift 4
    {
        printf '<?xml version="1.0" encoding="UTF-8"?>\n%s\n' "$open"
        jq -j --arg f "$field" '.rpm[] | .[$f] + "\n"' "$@"
        printf '%s\n' "$close"
    } > "$out/repodata/$name.xml"
    gzip -n -f -- "$out/repodata/$name.xml"
}
write primary primary \
    "<metadata xmlns=\"http://linux.duke.edu/metadata/common\" xmlns:rpm=\"http://linux.duke.edu/metadata/rpm\" packages=\"$count\">" \
    '</metadata>' "$@"
write filelists filelists \
    "<filelists xmlns=\"http://linux.duke.edu/metadata/filelists\" packages=\"$count\">" \
    '</filelists>' "$@"
write other other \
    "<otherdata xmlns=\"http://linux.duke.edu/metadata/other\" packages=\"$count\">" \
    '</otherdata>' "$@"

{
    printf '<?xml version="1.0" encoding="UTF-8"?>\n'
    printf '<repomd xmlns="http://linux.duke.edu/metadata/repo" xmlns:rpm="http://linux.duke.edu/metadata/rpm">\n'
    printf '  <revision>%s</revision>\n' "$timestamp"
    for name in primary filelists other; do
        gz=$out/repodata/$name.xml.gz
        printf '  <data type="%s">\n' "$name"
        printf '    <checksum type="sha256">%s</checksum>\n' "$(sha256sum -- "$gz" | cut -d' ' -f1)"
        printf '    <open-checksum type="sha256">%s</open-checksum>\n' "$(gzip -dc -- "$gz" | sha256sum | cut -d' ' -f1)"
        printf '    <location href="repodata/%s.xml.gz"/>\n' "$name"
        printf '    <timestamp>%s</timestamp>\n' "$timestamp"
        printf '    <size>%s</size>\n' "$(wc -c < "$gz" | tr -d ' ')"
        printf '    <open-size>%s</open-size>\n' "$(gzip -dc -- "$gz" | wc -c | tr -d ' ')"
        printf '  </data>\n'
    done
    printf '</repomd>\n'
} > "$out/repodata/repomd.xml"
printf 'assemble-rpm-repo: %s: %s packages\n' "$out" "$count"
