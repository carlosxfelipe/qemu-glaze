package main

import (
	"cmp"
	"embed"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"qemu-glaze/store"
	"qemu-glaze/vm"

	"github.com/crgimenes/glaze"
	"github.com/crgimenes/glaze/menu"
)

//go:embed all:ui assets/icon.png
var embeddedAssets embed.FS

func init() {
	runtime.LockOSThread()
}

// AppAPI exposes backend methods to JavaScript via glaze.BindMethods under the "api_" prefix.
type AppAPI struct {
	webview glaze.WebView
	mgr     *vm.Manager
	store   *store.Store
	events  *glaze.Events
}

func (a *AppAPI) SystemInfo() vm.SystemInfo {
	return a.mgr.GetSystemInfo()
}

func (a *AppAPI) BrowseISO() (vm.BrowseResponse, error) {
	selected, err := a.webview.OpenFile(glaze.FileDialogOptions{
		Title: "Selecionar Imagem ISO / Disco",
		Filters: []glaze.FileFilter{
			{Name: "Imagens de Disco & ISO", Extensions: []string{"iso", "img", "dmg", "raw", "qcow2"}},
			{Name: "Todos os Arquivos", Extensions: []string{"*"}},
		},
	})
	if err != nil {
		return vm.BrowseResponse{}, err
	}
	return vm.BrowseResponse{Path: selected}, nil
}

func (a *AppAPI) BrowseDisk() (vm.BrowseResponse, error) {
	selected, err := a.webview.OpenFile(glaze.FileDialogOptions{
		Title: "Selecionar Disco Virtual",
		Filters: []glaze.FileFilter{
			{Name: "Discos Virtuais QCOW2 / RAW", Extensions: []string{"qcow2", "img", "raw"}},
			{Name: "Todos os Arquivos", Extensions: []string{"*"}},
		},
	})
	if err != nil {
		return vm.BrowseResponse{}, err
	}
	return vm.BrowseResponse{Path: selected}, nil
}

func (a *AppAPI) OpenExternal(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	return openExternal(target)
}

func (a *AppAPI) CopyText(text string) error {
	return copyToClipboard(text)
}

func (a *AppAPI) ListMachines() (vm.VMListResponse, error) {
	var list []vm.VMConfig
	if _, err := a.store.Get("vms", &list); err != nil {
		return vm.VMListResponse{}, err
	}
	return vm.VMListResponse{
		Items:  list,
		States: a.mgr.GetAllStates(),
	}, nil
}

func (a *AppAPI) SaveMachine(req vm.SaveVMRequest) (vm.VMConfig, error) {
	item := req.VM
	if err := item.Validate(); err != nil {
		return vm.VMConfig{}, err
	}
	if err := a.mgr.EnsureDisk(&item); err != nil {
		return vm.VMConfig{}, err
	}

	var list []vm.VMConfig
	if _, err := a.store.Get("vms", &list); err != nil {
		return vm.VMConfig{}, err
	}

	if item.ID == "" {
		item.ID = fmt.Sprintf("vm_%d", time.Now().UnixMilli())
		item.CreatedAt = time.Now().Format("02/01/2006 15:04")
		list = append([]vm.VMConfig{item}, list...)
	} else {
		updated := false
		for i, cur := range list {
			if cur.ID == item.ID {
				item.CreatedAt = cmp.Or(item.CreatedAt, cur.CreatedAt)
				list[i] = item
				updated = true
				break
			}
		}
		if !updated {
			list = append([]vm.VMConfig{item}, list...)
		}
	}

	if err := a.store.Set("vms", list); err != nil {
		return vm.VMConfig{}, err
	}

	if a.events != nil {
		_ = a.events.Emit("vms:saved", item)
	}
	return item, nil
}

func (a *AppAPI) DeleteMachine(req vm.DeleteVMRequest) (map[string]string, error) {
	if strings.TrimSpace(req.ID) == "" {
		return nil, errors.New("ID da máquina é obrigatório")
	}

	_ = a.mgr.StopVM(req.ID, true)

	var list []vm.VMConfig
	_, _ = a.store.Get("vms", &list)

	filtered := make([]vm.VMConfig, 0, len(list))
	var target *vm.VMConfig
	for i := range list {
		if list[i].ID == req.ID {
			target = &list[i]
		} else {
			filtered = append(filtered, list[i])
		}
	}
	if err := a.store.Set("vms", filtered); err != nil {
		return nil, err
	}

	if req.DeleteDisk && target != nil && target.DiskPath != "" {
		_ = os.Remove(target.DiskPath)
	}

	if a.events != nil {
		_ = a.events.Emit("vms:deleted", map[string]string{"id": req.ID})
	}
	return map[string]string{"deleted": req.ID}, nil
}

func (a *AppAPI) StartMachine(req vm.StartVMRequest) (vm.VMState, error) {
	var targetCfg vm.VMConfig

	if req.AdHoc != nil {
		targetCfg = *req.AdHoc
		if err := targetCfg.Validate(); err != nil {
			return vm.VMState{}, err
		}
		if targetCfg.ID == "" {
			targetCfg.ID = fmt.Sprintf("quick_%d", time.Now().UnixMilli())
		}
	} else {
		var list []vm.VMConfig
		_, _ = a.store.Get("vms", &list)
		foundIdx := -1
		for i := range list {
			if list[i].ID == req.ID {
				targetCfg = list[i]
				foundIdx = i
				break
			}
		}
		if foundIdx == -1 {
			return vm.VMState{}, fmt.Errorf("máquina virtual com ID %q não encontrada", req.ID)
		}
		if targetCfg.DiskPath == "" && targetCfg.CreateDisk {
			if err := a.mgr.EnsureDisk(&targetCfg); err != nil {
				return vm.VMState{}, err
			}
			list[foundIdx] = targetCfg
			_ = a.store.Set("vms", list)
			if a.events != nil {
				_ = a.events.Emit("vms:saved", targetCfg)
			}
		}
	}

	state, err := a.mgr.StartVM(targetCfg)
	if err != nil {
		return vm.VMState{}, err
	}

	notifyDesktop("Máquina Iniciada", fmt.Sprintf("Máquina %q iniciada com sucesso (PID %d)", targetCfg.Name, state.PID))
	return *state, nil
}

func (a *AppAPI) StopMachine(req vm.StopVMRequest) (map[string]string, error) {
	if strings.TrimSpace(req.ID) == "" {
		return nil, errors.New("ID da máquina é obrigatório")
	}
	if err := a.mgr.StopVM(req.ID, req.Force); err != nil {
		return nil, err
	}

	notifyDesktop("Máquina Parada", "Máquina virtual parada com sucesso.")
	return map[string]string{"stopped": req.ID}, nil
}

func (a *AppAPI) BuildCommand(req vm.SaveVMRequest) (vm.BuildCommandResponse, error) {
	bin, args, err := a.mgr.BuildCommand(req.VM)
	if err != nil {
		return vm.BuildCommandResponse{}, err
	}

	var b strings.Builder
	b.WriteString(filepath.Base(bin))
	for _, arg := range args {
		b.WriteByte(' ')
		if len(arg) > 0 && (arg[0] == '-' || !strings.ContainsAny(arg, " \t\n\"'")) {
			b.WriteString(arg)
		} else {
			b.WriteByte('"')
			b.WriteString(arg)
			b.WriteByte('"')
		}
	}

	return vm.BuildCommandResponse{
		Binary:  bin,
		Args:    args,
		Command: b.String(),
	}, nil
}

func (a *AppAPI) CreateDisk(req vm.CreateDiskRequest) (vm.CreateDiskResponse, error) {
	diskPath, err := a.mgr.CreateDisk(req.Name, req.SizeGB)
	if err != nil {
		return vm.CreateDiskResponse{}, err
	}
	return vm.CreateDiskResponse{
		Path:   diskPath,
		SizeGB: req.SizeGB,
	}, nil
}

func (a *AppAPI) GetLogs(id string) vm.LogResponse {
	return vm.LogResponse{Lines: a.mgr.GetLogs(id)}
}

func serveAsset(req *glaze.SchemeRequest) *glaze.SchemeResponse {
	name := assetName(req.URL)
	if name == "icon.png" {
		data, err := embeddedAssets.ReadFile("assets/icon.png")
		if err != nil {
			return nil
		}
		return &glaze.SchemeResponse{Body: data, MIMEType: "image/png"}
	}

	data, err := embeddedAssets.ReadFile("ui/" + name)
	if err != nil {
		if path.Ext(name) != "" {
			return nil
		}
		data, err = embeddedAssets.ReadFile("ui/index.html")
		if err != nil {
			return nil
		}
		name = "index.html"
	}

	return &glaze.SchemeResponse{
		Body:     data,
		MIMEType: mimeTypeFor(name),
	}
}

func assetName(reqURL string) string {
	p := reqURL
	if u, err := url.Parse(reqURL); err == nil && u.Path != "" {
		p = u.Path
	}
	p = strings.TrimPrefix(path.Clean("/"+p), "/")
	if p == "" || p == "." {
		return "index.html"
	}
	return p
}

func mimeTypeFor(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func openExternal(target string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target).Start()
	case "linux":
		return exec.Command("xdg-open", target).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	default:
		return fmt.Errorf("plataforma não suportada: %s", runtime.GOOS)
	}
}

func notifyDesktop(title, body string) {
	go func() {
		switch runtime.GOOS {
		case "darwin":
			script := fmt.Sprintf("display notification %q with title %q", body, title)
			_ = exec.Command("osascript", "-e", script).Run()
		case "linux":
			if bin, err := exec.LookPath("notify-send"); err == nil {
				_ = exec.Command(bin, title, body).Run()
			}
		}
	}()
}

func copyToClipboard(text string) error {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	case "linux":
		for _, tool := range []struct {
			bin  string
			args []string
		}{
			{"wl-copy", nil},
			{"xclip", []string{"-selection", "clipboard"}},
			{"xsel", []string{"--clipboard", "--input"}},
		} {
			if p, err := exec.LookPath(tool.bin); err == nil {
				cmd := exec.Command(p, tool.args...)
				cmd.Stdin = strings.NewReader(text)
				return cmd.Run()
			}
		}
	}
	return errors.New("nenhum utilitário de área de transferência encontrado")
}

func main() {
	headless := os.Getenv("GLAZE_HEADLESS") == "1"
	debug := os.Getenv("GLAZE_DEBUG") == "1"

	st, err := store.New("com.carlosxfelipe.qemu-glaze", store.Options{
		EncryptionKey: "qemu-glaze-local-secret-key",
	})
	if err != nil {
		log.Printf("Aviso: fallback para store em memória: %v", err)
		st, _ = store.New("com.carlosxfelipe.qemu-glaze", store.Options{MemoryOnly: true})
	}

	if iconBytes, err := embeddedAssets.ReadFile("assets/icon.png"); err == nil {
		if err := glaze.SetAppIcon(iconBytes); err != nil && !errors.Is(err, glaze.ErrIconUnsupported) {
			log.Printf("Aviso ao definir ícone da aplicação: %v", err)
		}
	}

	var events *glaze.Events
	mgr := vm.NewManager(func(event string, data any) {
		if events != nil {
			_ = events.Emit(event, data)
		}
	})

	done := make(chan struct{})
	var w glaze.WebView

	w, err = glaze.NewWithOptions(glaze.Options{
		Debug:           debug,
		Hidden:          headless,
		HideUntilLoaded: !headless,
		SchemeHandlers: map[string]glaze.SchemeHandler{
			"app": serveAsset,
		},
		OnNewWindow: func(target string) {
			_ = openExternal(target)
		},
		OnClose: func() {
			close(done)
		},
	})
	if err != nil {
		log.Fatalf("glaze init: %v", err)
	}
	defer w.Destroy()

	events, err = glaze.NewEvents(w)
	if err != nil {
		log.Fatalf("glaze events: %v", err)
	}

	api := &AppAPI{
		webview: w,
		mgr:     mgr,
		store:   st,
		events:  events,
	}
	if _, err := glaze.BindMethods(w, "api", api); err != nil {
		log.Fatalf("glaze bind methods: %v", err)
	}

	m, err := menu.Set([]menu.Item{
		{Title: "QEMU Glaze", Submenu: []menu.Item{
			{Title: "Sobre o QEMU Glaze", OnClick: func() { _ = events.Emit("app:navigate", "about") }},
			{Separator: true},
			{Title: "Sair", Shortcut: "cmd+q", OnClick: w.Terminate},
		}},
		{Title: "Arquivo", Submenu: []menu.Item{
			{Title: "Nova Máquina", Shortcut: "cmd+n", OnClick: func() { _ = events.Emit("app:navigate", "new-vm") }},
			{Title: "Atualizar Lista", Shortcut: "cmd+r", OnClick: func() { _ = events.Emit("app:refresh") }},
		}},
		{Title: "Editar", Submenu: []menu.Item{
			{Title: "Desfazer", Shortcut: "cmd+z", Selector: "undo:"},
			{Title: "Refazer", Shortcut: "cmd+shift+z", Selector: "redo:"},
			{Separator: true},
			{Title: "Recortar", Shortcut: "cmd+x", Selector: "cut:"},
			{Title: "Copiar", Shortcut: "cmd+c", Selector: "copy:"},
			{Title: "Colar", Shortcut: "cmd+v", Selector: "paste:"},
			{Title: "Selecionar Tudo", Shortcut: "cmd+a", Selector: "selectAll:"},
		}},
		{Title: "Navegação", Submenu: []menu.Item{
			{Title: "Minhas Máquinas", Shortcut: "cmd+1", OnClick: func() { _ = events.Emit("app:navigate", "vms") }},
			{Title: "Nova Máquina", Shortcut: "cmd+2", OnClick: func() { _ = events.Emit("app:navigate", "new-vm") }},
			{Title: "Console & Logs", Shortcut: "cmd+3", OnClick: func() { _ = events.Emit("app:navigate", "logs") }},
			{Title: "Status do QEMU", Shortcut: "cmd+4", OnClick: func() { _ = events.Emit("app:navigate", "system") }},
		}},
	}, menu.Options{Window: w.Window()})
	if err != nil && !errors.Is(err, menu.ErrUnsupported) {
		log.Printf("Aviso ao instalar menu nativo: %v", err)
	}
	if m != nil {
		defer m.Release()
	}

	w.SetTitle("QEMU Glaze — Virtual Machine Manager")
	w.SetSize(1180, 780, glaze.HintNone)
	w.SetSize(960, 680, glaze.HintMin)
	w.Navigate("app://home/index.html")

	// Background telemetry ticker for running VM uptime updates.
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				states := mgr.GetAllStates()
				if len(states) > 0 {
					_ = events.Emit("vms:telemetry", states)
				}
			}
		}
	}()

	log.Printf("🖥️ QEMU Glaze pronto (origem portless app://home/index.html)")
	w.Run()
	mgr.StopAll()
}
