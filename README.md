# QEMU Glaze — Gerenciador de Máquinas Virtuais

Aplicativo desktop para criar, configurar e gerenciar máquinas virtuais QEMU, escrito em **Go puro (`CGO_ENABLED=0`)** com o [**Glaze**](https://github.com/crgimenes/glaze) (`WKWebView` no macOS e `WebKitGTK` no Linux via `purego`).

A interface web embarcada é servida diretamente pelo protocolo customizado `app://home/index.html`, sem abrir portas TCP locais, comunicando-se com o backend Go via `glaze.BindMethods` e eventos em tempo real (`glaze.NewEvents`).

## O que o projeto faz

- **Gerenciamento de VMs**: crie, edite, liste, inicie, pare (normal ou forçado) e exclua máquinas virtuais, com opção de remover o disco virtual associado.
- **Quick Boot**: inicie uma máquina virtual rapidamente sem precisar salvá-la previamente.
- **Suporte a múltiplas arquiteturas**: configure VMs `x86_64` e `aarch64` com 1 a 64 núcleos, 256 MB a 131072 MB de RAM, aceleração de hardware e argumentos extras personalizados.
- **Discos virtuais e ISOs**: crie discos `qcow2` (via `qemu-img` em `~/.qemu-glaze/disks`) e selecione imagens ISO ou discos existentes usando diálogos nativos do sistema operacional.
- **Pré-visualização de comando QEMU**: visualize e copie para a área de transferência o comando QEMU gerado antes de executar a VM.
- **Console, Logs e Telemetria**: acompanhe status, PID, tempo de atividade (uptime) em tempo real e logs de saída de cada máquina.
- **Status do QEMU e Sistema**: detecta automaticamente CPUs, memória e espaço em disco do host, versão do QEMU, binários (`qemu-system-x86_64`, `qemu-system-aarch64`, `qemu-img`), aceleradores e firmwares EDK2.
- **Menu nativo e notificações**: barra de menus nativa com atalhos (`Cmd/Ctrl+1..4`, `Cmd/Ctrl+N`, `Cmd/Ctrl+R`, `Cmd/Ctrl+Q`) e notificações do sistema ao iniciar ou parar VMs.
- **Persistência criptografada**: configurações das VMs salvas localmente com AES-256-GCM e escrita atômica (`com.carlosxfelipe.qemu-glaze`).

## Pré-requisitos

- **Go** `1.27+`
- **QEMU** instalado no `PATH` (`qemu-system-x86_64` / `qemu-system-aarch64` e `qemu-img`)
- **Linux**: `WebKitGTK` para execução da interface; opcionalmente `appimagetool` (para gerar `.AppImage`), `notify-send` (notificações) e `wl-copy` / `xclip` / `xsel` (área de transferência)

## Comandos Principais

### Executar, testar e validar

```bash
go mod tidy
go vet ./...
go test ./...
go run .
```

Variáveis de ambiente úteis durante o desenvolvimento:

- `GLAZE_DEBUG=1`: habilita o modo debug / inspetor da WebView.
- `GLAZE_HEADLESS=1`: inicia a janela oculta.

## Scripts

### `update-glaze.sh`

Gerencia a dependência do `github.com/crgimenes/glaze` no `go.mod` e executa `go mod tidy`, `go vet ./...` e `go test ./...`:

```bash
# Atualiza para o commit mais recente da branch trunk no GitHub
./update-glaze.sh

# Alterna para o repositório local em ../glaze (via diretiva replace)
./update-glaze.sh --local
```

### `fmt.sh`

Formata todo o código do repositório:

```bash
./fmt.sh
```

- Código Go via `go fmt ./...`
- Arquivos `.js`, `.css`, `.html` e `.md` via `prettier` (ou `npx prettier`)
- Shell scripts `.sh` via `shfmt` (ou `uvx --from shfmt-py shfmt`)

## Empacotamento e Assets

### macOS (`.app` com `CGO_ENABLED=0`)

Gera o bundle `QEMU Glaze.app`:

```bash
go run build_mac.go
open "QEMU Glaze.app"
```

### Linux (`AppDir` / `AppImage` com `CGO_ENABLED=0`)

Gera `QEMU_Glaze-1.0.0-<arch>.AppImage` (`x86_64`, `aarch64`, `i686` ou `armhf`), baixando automaticamente o `appimagetool` em cache caso não esteja no `PATH`:

```bash
go run build_linux.go
# Opcional: manter o diretório temporário .AppDir após gerar o AppImage
go run build_linux.go --keep-appdir
```

### Gerar ícones

Regenera `assets/icon.png` e `assets/app.icns` a partir de `assets/icon_source.svg`:

```bash
go run assets/generate_icon.go
```

## Estrutura do Projeto

- `main.go` — inicialização da janela Glaze, protocolo `app://`, API exposta ao frontend (`AppAPI`), menu nativo e telemetria.
- `vm/` — gerenciamento do ciclo de vida dos processos QEMU, construção de comandos, criação de discos e detecção de ambiente.
- `store/` — persistência local criptografada com AES-256-GCM.
- `ui/` — interface web embarcada (`index.html`, `app.css`, `app.js`).
- `assets/` — ícone fonte SVG, ícones compilados e script `generate_icon.go`.
- `build_mac.go` / `build_linux.go` — empacotamento nativo para macOS e Linux.
- `AGENTS.md` — diretrizes de interface (GNOME HIG / GTK 4.x+) e arquitetura para colaboradores e agentes.

## Licença

Este projeto está licenciado sob a [Licença MIT](LICENSE).

Copyright (c) 2026 **Carlos Felipe Araújo**.
