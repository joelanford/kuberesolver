#!/usr/bin/env bash

if [[ "$#" -ne "1" ]]; then
	echo "Usage: $0 <subscriptionName>"
	exit 1
fi

sub=$1
packageName=$(kubectl get subscription $sub -o jsonpath={.spec.package})
nextVersion=$(kubectl get subscription $sub -o jsonpath={.status.upgradeTo})

if [[ -z "$nextVersion" ]]; then
	echo "No upgrade version exists for subscription \"$sub\""
	exit 1
fi

cat <<EOF | kubectl apply -f -
apiVersion: olm.operatorframework.io/v1
kind: Operator
metadata:
  name: $sub
spec:
  package: $sub
  version: $nextVersion
EOF
