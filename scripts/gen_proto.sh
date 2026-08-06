#!/usr/bin/env bash
set -euo pipefail
PATH="${HOME}/go/bin:${PATH}"
PROTO_DIR="api"
GO_OUT_DIR="gen"
FULL_MODULE=$(awk '/^module/ {print $2}' go.mod)

if [ -z "${INC:-}" ]; then
  if ! go list -m -f '{{.Dir}}' github.com/googleapis/googleapis >/dev/null 2>&1; then
    go get github.com/googleapis/googleapis@latest
  fi
  INC=$(go list -m -f '{{.Dir}}' github.com/googleapis/googleapis)
fi

rm -rf "${GO_OUT_DIR}"
mkdir -p "$GO_OUT_DIR"

while IFS= read -r file; do
  [ -z "$file" ] && continue
  mkdir -p "$GO_OUT_DIR/$(dirname "${file#$PROTO_DIR/}")"
  protoc --go_out="$GO_OUT_DIR" --go_opt=module="$FULL_MODULE/$GO_OUT_DIR" \
         --go-grpc_out="$GO_OUT_DIR" --go-grpc_opt=module="$FULL_MODULE/$GO_OUT_DIR" \
         --grpc-gateway_out="$GO_OUT_DIR" --grpc-gateway_opt=module="$FULL_MODULE/$GO_OUT_DIR" \
         --grpc-gateway_opt=logtostderr=true \
         -I. -I"$PROTO_DIR/.." -I"${INC}" \
         "$file"
done < <(/usr/bin/find "$PROTO_DIR" -name '*.proto' | grep -E 'api/(auth|notes|common)/v1' | sort)

go mod tidy
