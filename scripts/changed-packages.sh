#!/bin/bash
# Prints ./dir/... patterns for the Go packages that changed vs HEAD, plus
# every package that imports one of them, space-separated on stdout. Used by
# `make check` to scope lint/test to a change's blast radius instead of the
# whole module. Relative patterns (not import paths) because golangci-lint
# resolves positional args against the filesystem, not the module graph.
set -euo pipefail

changed_dirs=$( { git diff --name-only HEAD -- '*.go'; git ls-files --others --exclude-standard -- '*.go'; } | xargs -r -n1 dirname | sort -u)
if [ -z "$changed_dirs" ]; then
	exit 0
fi

changed_pkgs=""
for dir in $changed_dirs; do
	pkg=$(go list "./$dir" 2>/dev/null) || continue
	changed_pkgs="$changed_pkgs $pkg"
done
if [ -z "$changed_pkgs" ]; then
	exit 0
fi

target_import_paths="$changed_pkgs"
for pkg in $(go list ./...); do
	deps=$(go list -f '{{join .Deps " "}}' "$pkg" 2>/dev/null) || continue
	for changed in $changed_pkgs; do
		if [[ " $deps " == *" $changed "* ]]; then
			target_import_paths="$target_import_paths $pkg"
			break
		fi
	done
done

for pkg in $(echo "$target_import_paths" | tr ' ' '\n' | sort -u); do
	dir=$(go list -f '{{.Dir}}' "$pkg")
	rel=$(realpath --relative-to=. "$dir")
	if [ "$rel" = "." ]; then
		echo -n "./ "
	else
		echo -n "./$rel/... "
	fi
done
