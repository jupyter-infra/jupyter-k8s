#!/bin/bash
set -e

WORKSPACE_NAME="space-poc"
NAMESPACE="default"

# Get fresh JWT token
TOKEN=$(kubectl create --raw "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/${NAMESPACE}/workspaceconnections" -f - 2>/dev/null <<EOF | python3 -c "import sys,json; url=json.load(sys.stdin)['status']['workspaceConnectionUrl']; print(url.split('token=')[1])"
{"apiVersion":"connection.workspace.jupyter.org/v1alpha1","kind":"WorkspaceConnection","metadata":{"namespace":"${NAMESPACE}"},"spec":{"workspaceName":"${WORKSPACE_NAME}","workspaceConnectionType":"web-ui"}}
EOF
)

if [ -z "$TOKEN" ]; then
    echo "Error: Failed to get token" >&2
    exit 1
fi

HOST=$(kubectl get ingressroute "authorized-route-${WORKSPACE_NAME}" -n "${NAMESPACE}" -o jsonpath='{.spec.routes[0].match}' 2>/dev/null | grep -o '`[^`]*`' | head -1 | tr -d '`')

exec websocat --binary asyncstdio: "wss://${HOST}/ssh-ws?token=${TOKEN}"
