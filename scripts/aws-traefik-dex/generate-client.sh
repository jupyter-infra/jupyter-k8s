#!/bin/bash

set -e

CLUSTER_NAME=$1
DEX_URL=$2
AWS_REGION=${3:-us-west-2}
PORT=${4:-9800}
OUT_FILEPATH=${5:-dist/users-scripts/set-kubeconfig.sh}

API_ENDPOINT=$(aws eks describe-cluster --region ${AWS_REGION} --name ${CLUSTER_NAME} --query "cluster.endpoint" --output text)
API_CERT=$(aws eks describe-cluster --region ${AWS_REGION} --name ${CLUSTER_NAME} --query "cluster.certificateAuthority.data" --output text)

mkdir -p $(dirname ${OUT_FILEPATH})

tee ${OUT_FILEPATH} > /dev/null << EOF
#!/bin/bash
set -e

# install required cli tool
brew install kubelogin

# kill any process running on the target port
PID=\$(lsof -i :${PORT} | awk 'NR>1 {print \$2}')

if [ -n "\$PID" ]; then
    echo "Terminating existing process running on port ${PORT}"
    kill -9 \$PID
fi

mkdir -p /tmp/eks-certs
echo ${API_CERT} | base64 --decode > /tmp/eks-certs/${CLUSTER_NAME}-ca.crt

kubectl config set-cluster remote-aws-cluster \
    --embed-certs \
    --certificate-authority=/tmp/eks-certs/${CLUSTER_NAME}-ca.crt \
    --server ${API_ENDPOINT}

kubectl config set-credentials github-user \
  --exec-api-version=client.authentication.k8s.io/v1 \
  --exec-interactive-mode=IfAvailable \
  --exec-command=kubectl \
  --exec-arg=oidc-login \
  --exec-arg=get-token \
  --exec-arg="--oidc-issuer-url=${DEX_URL}" \
  --exec-arg="--oidc-client-id=kubectl-oidc" \
  --exec-arg="--listen-address=localhost:${PORT}" \
  --exec-arg="--oidc-extra-scope=profile" \
  --exec-arg="--oidc-extra-scope=groups"

kubectl config set-context remote-aws-github \
    --cluster=remote-aws-cluster \
    --user=github-user

kubectl config use-context remote-aws-github
EOF

chmod +x ${OUT_FILEPATH}
