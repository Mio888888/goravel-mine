#!/usr/bin/env sh
set -eu

CHART_DIR="${CHART_DIR:-deploy/helm/goravel-mine}"
K8S_DIR="${K8S_DIR:-deploy/k8s}"
OUT_DIR="${OUT_DIR:-tmp/deploy-verify}"
RELEASE_NAME="${RELEASE_NAME:-goravel-mine}"
NAMESPACE="${NAMESPACE:-goravel-mine}"
IMAGE_REPOSITORY="${IMAGE_REPOSITORY:-ghcr.io/example/goravel-mine}"
IMAGE_TAG="${IMAGE_TAG:-ci}"
SECRET_NAME="${SECRET_NAME:-goravel-mine-secret}"
KUBECTL_DRY_RUN="${KUBECTL_DRY_RUN:-server}"
KUBECTL_VALIDATE="${KUBECTL_VALIDATE:-strict}"
ENSURE_NAMESPACE="${ENSURE_NAMESPACE:-false}"

mkdir -p "$OUT_DIR"

helm lint "$CHART_DIR" \
  --set image.repository="$IMAGE_REPOSITORY" \
  --set image.tag="$IMAGE_TAG" \
  --set secret.existingSecret="$SECRET_NAME"

helm template "$RELEASE_NAME" "$CHART_DIR" \
  --namespace "$NAMESPACE" \
  --set image.repository="$IMAGE_REPOSITORY" \
  --set image.tag="$IMAGE_TAG" \
  --set secret.existingSecret="$SECRET_NAME" \
  > "$OUT_DIR/helm-template.yaml"

helm template "$RELEASE_NAME" "$CHART_DIR" \
  --namespace "$NAMESPACE" \
  --set image.repository="$IMAGE_REPOSITORY" \
  --set image.tag="$IMAGE_TAG" \
  --set secret.existingSecret="$SECRET_NAME" \
  --set migration.enabled=true \
  --set backup.enabled=true \
  > "$OUT_DIR/helm-template-ops.yaml"

helm template "$RELEASE_NAME" "$CHART_DIR" \
  --namespace "$NAMESPACE" \
  -f "$CHART_DIR/values-ha-test.yaml" \
  --set image.repository="$IMAGE_REPOSITORY" \
  --set image.tag="$IMAGE_TAG" \
  --set secret.existingSecret="$SECRET_NAME" \
  > "$OUT_DIR/helm-template-ha.yaml"

scripts/verify-helm-ha.sh "$OUT_DIR/helm-template-ha.yaml"

if [ "$ENSURE_NAMESPACE" = "true" ]; then
  kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
fi

kubectl apply --dry-run="$KUBECTL_DRY_RUN" --validate="$KUBECTL_VALIDATE" -f "$K8S_DIR/"
kubectl apply --dry-run="$KUBECTL_DRY_RUN" --validate="$KUBECTL_VALIDATE" --namespace "$NAMESPACE" -f "$OUT_DIR/helm-template.yaml"
kubectl apply --dry-run="$KUBECTL_DRY_RUN" --validate="$KUBECTL_VALIDATE" --namespace "$NAMESPACE" -f "$OUT_DIR/helm-template-ops.yaml"
kubectl apply --dry-run="$KUBECTL_DRY_RUN" --validate="$KUBECTL_VALIDATE" --namespace "$NAMESPACE" -f "$OUT_DIR/helm-template-ha.yaml"
