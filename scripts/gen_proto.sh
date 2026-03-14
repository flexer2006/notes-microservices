#!/usr/bin/env bash
set -euo pipefail
PROTO_DIR="api"
GO_OUT_DIR="gen"
FULL_MODULE=$(awk '/^module/ {print $2}' go.mod)
if [ -z "${INC:-}" ]; then
  if go list -m -f '{{.Dir}}' github.com/googleapis/googleapis >/dev/null 2>&1; then
    INC=$(go list -m -f '{{.Dir}}' github.com/googleapis/googleapis)
  fi
fi
rm -rf "${GO_OUT_DIR%/*}" && mkdir -p "$GO_OUT_DIR"
find "$PROTO_DIR" -name "*.proto" | grep -E 'api/(auth|notes|common)/v1' | while read -r file; do
  mkdir -p "$GO_OUT_DIR/$(dirname "${file#$PROTO_DIR/}")"
  extra_inc_args=()
  if [ -n "${INC:-}" ]; then
    extra_inc_args+=("-I${INC}")
  fi
  protoc --go_out="$GO_OUT_DIR" --go_opt=module="$FULL_MODULE/$GO_OUT_DIR" \
         --go-grpc_out="$GO_OUT_DIR" --go-grpc_opt=module="$FULL_MODULE/$GO_OUT_DIR" \
         --grpc-gateway_out="$GO_OUT_DIR" --grpc-gateway_opt=module="$FULL_MODULE/$GO_OUT_DIR" \
         --grpc-gateway_opt=logtostderr=true \
         -I. -I"$PROTO_DIR/.." "${extra_inc_args[@]}" "$file" || echo -e "\e[31mОшибка генерации: $file\e[0m"
done
if [ -n "${TEMP_DIR:-}" ]; then
  rm -rf "$TEMP_DIR"
fi
go mod tidy