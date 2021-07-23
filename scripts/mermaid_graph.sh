#!/usr/bin/env bash

set -e

if [[ "$#" -ne "2" ]]; then
	echo "Usage: $0 <indexName> <packageName>"
	exit 1
fi

index=$(kubectl get index $1 -o json)
pkg=$(echo "$index" | jq --arg pkg $2 -r '.spec.packages[] | select(.name == $pkg)')

if [[ -z "$pkg" ]]; then
	echo "package \"$2\" not found in index \"$1\""
	exit 1
fi

nodes=$(echo "$pkg" | jq -r '.bundles[] | "  " + .version')
edges=$(echo "$pkg" | jq -r '.bundles[] | {from: .upgradesFrom[], to: .version} | "  " + .from + "-->" + .to')

echo "graph BT"
echo "$nodes"
echo "$edges"
