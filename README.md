# QEMU Glaze — Gerenciador de Máquinas Virtuais

Versão do **QEMU Brisa** recriada sobre o [**Glaze**](https://github.com/crgimenes/glaze), rodando **100% em Go puro (`CGO_ENABLED=0`)** no macOS e Linux, sem abrir portas TCP locais (via custom scheme `app://`) e com menu nativo (`glaze/menu`).

## Diferenças arquiteturais em relação ao `qemu-brisa`

- **Zero CGo em todas as plataformas**: usa `WKWebView` (macOS) e `WebKitGTK` (Linux) via `purego`.
- **Zero portas TCP**: os assets embarcados (`ui/index.html`, `ui/theme.css`, `ui/app.css`, `ui/app.js`, `assets/icon.png`) são servidos pelo protocolo customizado `app://home/...` (Secure Context).
- **Bridge direta**: usa `glaze.BindMethods` (`api_*`) e `glaze.NewEvents` (`glaze.events`) em vez de HTTP RPC + SSE.
- **Diálogos e Menu nativos**: usa `WebView.OpenFile` (`NSOpenPanel` / `GtkFileChooserNative`) e `glaze/menu` (`NSMenu` / atalhos GTK).
- **Store criptografado local**: pacote `store` em `encoding/json/v2` + AES-256-GCM com escrita atômica em `~/.config` / `~/Library/Application Support/com.carlosxfelipe.qemu-glaze/store.json`.
- **Discos virtuais**: armazenados em `~/.qemu-glaze/disks`.

## Executar e Testar

```bash
go mod tidy
go test ./...
go run .
```

## Atualizar o Glaze e Formatar

- **Alternar para o Glaze local (`../glaze`) ou atualizar para o `main` remoto**:

  ```bash
  ./update-glaze.sh --local
  ./update-glaze.sh
  ```

- **Formatar código Go, JS e shell scripts**:

  ```bash
  ./fmt.sh
  ```

## Empacotamento

- **macOS (`.app` com `CGO_ENABLED=0`)**:

  ```bash
  go run build_mac.go
  open "QEMU Glaze.app"
  ```

- **Linux (`AppDir` / `AppImage` com `CGO_ENABLED=0`)**:

  ```bash
  go run build_linux.go
  ```
