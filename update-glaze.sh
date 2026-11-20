#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

if [[ "$1" == "--local" ]]; then
  echo "🧊 Alternando para desenvolvimento local do Glaze (../glaze)..."
  go mod edit -replace github.com/crgimenes/glaze=../glaze
  go mod tidy
  echo "🧪 Verificando compilação e testes com Glaze local..."
  go vet ./...
  go test ./...
  echo "✅ Agora usando Glaze local de: ../glaze"
  exit 0
fi

echo "🧊 Atualizando github.com/crgimenes/glaze para o commit mais recente no main..."

# Se houver diretiva de replace local ou require v0.0.0 inválido, limpa para buscar do GitHub
if grep -q "replace github.com/crgimenes/glaze" go.mod 2>/dev/null; then
  echo "📦 Removendo diretiva replace para buscar do GitHub remoto..."
  go mod edit -dropreplace github.com/crgimenes/glaze
fi

if grep -q "github.com/crgimenes/glaze v0.0.0$" go.mod 2>/dev/null; then
  go mod edit -droprequire github.com/crgimenes/glaze
fi

GOPROXY=direct go get github.com/crgimenes/glaze@main
go mod tidy

echo "🧪 Executando vet e testes..."
go vet ./...
go test ./...

VERSION=$(grep 'github.com/crgimenes/glaze' go.mod | grep -v 'replace' | awk '{print $NF}')
echo "✅ Glaze atualizado com sucesso para: ${VERSION}"
echo "💡 (Para voltar a usar o código local do Glaze, use: ./update-glaze.sh --local)"
