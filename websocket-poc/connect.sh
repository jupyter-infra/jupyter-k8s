#!/bin/bash
set -e

WORKSPACE_NAME="${1:-space-poc}"
NAMESPACE="${2:-default}"
DOMAIN="parker1.shared-parker.jjtowner.people.aws.dev"

# Get fresh JWT token from Extension API
TOKEN=$(kubectl create --raw "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/${NAMESPACE}/workspaceconnections" -f - 2>/dev/null <<EOF | python3 -c "import sys,json; url=json.load(sys.stdin)['status']['workspaceConnectionUrl']; print(url.split('token=')[1])"
{"apiVersion":"connection.workspace.jupyter.org/v1alpha1","kind":"WorkspaceConnection","metadata":{"namespace":"${NAMESPACE}"},"spec":{"workspaceName":"${WORKSPACE_NAME}","workspaceConnectionType":"web-ui"}}
EOF
)

if [ -z "$TOKEN" ]; then
    echo "Error: Failed to get token" >&2
    exit 1
fi

# Get workspace subdomain from IngressRoute
HOST=$(kubectl get ingressroute "authorized-route-${WORKSPACE_NAME}" -n "${NAMESPACE}" -o jsonpath='{.spec.routes[0].match}' 2>/dev/null | grep -o '`[^`]*`' | head -1 | tr -d '`')

if [ -z "$HOST" ]; then
    echo "Error: Could not find IngressRoute for workspace ${WORKSPACE_NAME}" >&2
    exit 1
fi

WSS_URL="wss://${HOST}/ssh-ws?token=${TOKEN}"

# Write the proxy script
cat > /tmp/ws-proxy.sh << PROXY
#!/bin/bash
exec websocat --binary asyncstdio: "${WSS_URL}"
PROXY
chmod +x /tmp/ws-proxy.sh

echo "Connecting to ${WORKSPACE_NAME} via WebSocket..."
exec ssh -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o "ProxyCommand=/tmp/ws-proxy.sh" \
    sagemaker-user@dummy
