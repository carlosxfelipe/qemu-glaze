package vm

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ProcessHandle tracks a running QEMU child process and its recent output lines.
type ProcessHandle struct {
	Config    VMConfig
	Cmd       *exec.Cmd
	StartedAt time.Time
	Cancel    chan struct{}
	Logs      []string
}

// Manager coordinates QEMU binary discovery, command building, and VM lifecycles.
type Manager struct {
	mu        sync.RWMutex
	processes map[string]*ProcessHandle
	states    map[string]VMState
	disksDir  string
	onEvent   func(event string, data any)
}

// NewManager creates a Manager storing automatic QCOW2 disks under ~/.qemu-glaze/disks.
func NewManager(onEvent func(event string, data any)) *Manager {
	home, _ := os.UserHomeDir()
	disksDir := filepath.Join(home, ".qemu-glaze", "disks")
	_ = os.MkdirAll(disksDir, 0755)

	return &Manager{
		processes: make(map[string]*ProcessHandle),
		states:    make(map[string]VMState),
		disksDir:  disksDir,
		onEvent:   onEvent,
	}
}

// DisksDir returns the directory used for automatically created QCOW2 disks.
func (m *Manager) DisksDir() string {
	return m.disksDir
}

// DefaultDiskPath returns the standard QCOW2 disk path for a VM name.
func (m *Manager) DefaultDiskPath(name string) string {
	diskName := fmt.Sprintf("%s.qcow2", sanitizeFileName(name))
	return filepath.Join(m.disksDir, diskName)
}

// EnsureDisk creates the automatic QCOW2 disk if requested and sets cfg.DiskPath.
func (m *Manager) EnsureDisk(cfg *VMConfig) error {
	if cfg == nil {
		return nil
	}
	if cfg.ISOPath == "" && strings.HasSuffix(strings.ToLower(cfg.DiskPath), ".iso") {
		cfg.ISOPath = cfg.DiskPath
		cfg.DiskPath = ""
	}
	if cfg.CreateDisk && cfg.DiskPath == "" {
		diskName := fmt.Sprintf("%s.qcow2", sanitizeFileName(cfg.Name))
		size := cfg.DiskSizeGB
		if size <= 0 {
			size = 20
		}
		path, err := m.CreateDisk(diskName, size)
		if err != nil {
			return err
		}
		cfg.DiskPath = path
	}
	return nil
}

// LocateBinary looks for an executable in PATH or standard Unix/Homebrew directories.
func LocateBinary(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	candidates := []string{
		filepath.Join("/opt/homebrew/bin", name),
		filepath.Join("/usr/local/bin", name),
		filepath.Join("/usr/bin", name),
		filepath.Join("/bin", name),
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

// LocateFirmware searches for EDK2 / OVMF UEFI firmware files.
func LocateFirmware(filename string) string {
	candidates := []string{
		filepath.Join("/opt/homebrew/share/qemu", filename),
		filepath.Join("/usr/local/share/qemu", filename),
		filepath.Join("/usr/share/qemu", filename),
		filepath.Join("/usr/share/edk2-ovmf", filename),
		filepath.Join("/usr/share/OVMF", filename),
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

// GetHostMemoryMB queries total host RAM in MB.
func GetHostMemoryMB() int {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err == nil {
			totalBytes, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
			if err == nil && totalBytes > 0 {
				return int(totalBytes / (1024 * 1024))
			}
		}
	case "linux":
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			for line := range strings.SplitSeq(string(data), "\n") {
				if rest, ok := strings.CutPrefix(line, "MemTotal:"); ok {
					fields := strings.Fields(rest)
					if len(fields) >= 1 {
						kb, err := strconv.ParseInt(fields[0], 10, 64)
						if err == nil && kb > 0 {
							return int(kb / 1024)
						}
					}
				}
			}
		}
	}
	return 8192
}

// GetDiskSpaceGB queries available and total disk space in GB for a directory.
func GetDiskSpaceGB(path string) (freeGB int, totalGB int) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	freeBytes := uint64(stat.Bavail) * uint64(stat.Bsize)
	totalBytes := uint64(stat.Blocks) * uint64(stat.Bsize)
	return int(freeBytes / (1024 * 1024 * 1024)), int(totalBytes / (1024 * 1024 * 1024))
}

// GetSystemInfo detects host capabilities and QEMU binaries.
func (m *Manager) GetSystemInfo() SystemInfo {
	qemuX86 := LocateBinary("qemu-system-x86_64")
	qemuArm := LocateBinary("qemu-system-aarch64")
	qemuImg := LocateBinary("qemu-img")

	version := "Not found"
	targetBin := cmp.Or(qemuX86, qemuArm)
	if targetBin != "" {
		out, err := exec.Command(targetBin, "--version").Output()
		if err == nil {
			firstLine, _, _ := strings.Cut(string(out), "\n")
			version = strings.TrimSpace(firstLine)
		}
	}

	accels := []string{"tcg"}
	switch runtime.GOOS {
	case "darwin":
		accels = append(accels, "hvf")
	case "linux":
		accels = append(accels, "kvm")
	}

	freeDiskGB, totalDiskGB := GetDiskSpaceGB(m.disksDir)

	return SystemInfo{
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		HostCPUs:        runtime.NumCPU(),
		HostMemoryMB:    GetHostMemoryMB(),
		HostDiskFreeGB:  freeDiskGB,
		HostDiskTotalGB: totalDiskGB,
		QemuVersion:     version,
		QemuX86Path:     qemuX86,
		QemuArmPath:     qemuArm,
		QemuImgPath:     qemuImg,
		Accelerators:    accels,
		Edk2ArmPath:     LocateFirmware("edk2-aarch64-code.fd"),
		Edk2X86Path:     LocateFirmware("edk2-x86_64-code.fd"),
		DisksDir:        m.disksDir,
	}
}

// CreateDisk creates a QCOW2 virtual disk image of sizeGB gigabytes.
func (m *Manager) CreateDisk(name string, sizeGB int) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("nome do disco é obrigatório")
	}
	if sizeGB < 1 || sizeGB > 1000 {
		return "", fmt.Errorf("tamanho do disco inválido (%d GB): deve ficar entre 1 e 1000 GB", sizeGB)
	}

	qemuImg := LocateBinary("qemu-img")
	if qemuImg == "" {
		return "", errors.New("qemu-img não encontrado. Certifique-se de que o QEMU está instalado")
	}

	if !strings.HasSuffix(name, ".qcow2") && !strings.HasSuffix(name, ".img") {
		name += ".qcow2"
	}
	targetPath := filepath.Join(m.disksDir, name)

	// Avoid overwriting an existing disk image.
	if _, err := os.Stat(targetPath); err == nil {
		return targetPath, nil
	}

	sizeStr := fmt.Sprintf("%dG", sizeGB)
	cmd := exec.Command(qemuImg, "create", "-f", "qcow2", targetPath, sizeStr)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("falha ao criar disco: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	return targetPath, nil
}

// BuildCommand generates the exact executable path and arguments for a VM.
func (m *Manager) BuildCommand(cfg VMConfig) (string, []string, error) {
	if cfg.ISOPath == "" && strings.HasSuffix(strings.ToLower(cfg.DiskPath), ".iso") {
		cfg.ISOPath = cfg.DiskPath
		cfg.DiskPath = ""
	}

	if cfg.CreateDisk && cfg.DiskPath == "" {
		cfg.DiskPath = m.DefaultDiskPath(cfg.Name)
	}

	diskFormat := "qcow2"
	if lower := strings.ToLower(cfg.DiskPath); strings.HasSuffix(lower, ".raw") || strings.HasSuffix(lower, ".img") || strings.HasSuffix(lower, ".iso") {
		diskFormat = "raw"
	}

	var bin string
	switch cfg.Arch {
	case "aarch64":
		bin = LocateBinary("qemu-system-aarch64")
		if bin == "" {
			return "", nil, errors.New("qemu-system-aarch64 não encontrado no sistema")
		}
	default:
		bin = LocateBinary("qemu-system-x86_64")
		if bin == "" {
			return "", nil, errors.New("qemu-system-x86_64 não encontrado no sistema")
		}
	}

	mem := cfg.MemoryMB
	if mem <= 0 {
		mem = 2048
	}
	cores := cfg.Cores
	if cores <= 0 {
		cores = 2
	}

	args := []string{
		"-m", fmt.Sprintf("%dM", mem),
		"-smp", fmt.Sprintf("%d", cores),
	}

	isDarwinArm := runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
	isDarwinX86 := runtime.GOOS == "darwin" && runtime.GOARCH == "amd64"
	isLinuxArm := runtime.GOOS == "linux" && runtime.GOARCH == "arm64"
	isLinuxX86 := runtime.GOOS == "linux" && runtime.GOARCH == "amd64"

	if cfg.Arch == "aarch64" {
		args = append(args, "-M", "virt,highmem=on")
		switch {
		case cfg.EnableAccel && isDarwinArm:
			args = append(args, "-accel", "hvf", "-cpu", "host")
		case cfg.EnableAccel && isLinuxArm:
			args = append(args, "-accel", "kvm", "-cpu", "host")
		default:
			args = append(args, "-accel", "tcg,thread=multi,tb-size=512", "-cpu", "cortex-a72")
		}

		if edk2 := LocateFirmware("edk2-aarch64-code.fd"); edk2 != "" {
			args = append(args, "-bios", edk2)
		}

		if runtime.GOOS == "darwin" {
			args = append(args, "-display", "cocoa")
		}
		args = append(args,
			"-device", "virtio-gpu-pci",
			"-device", "qemu-xhci",
			"-device", "usb-kbd",
			"-device", "usb-tablet",
			"-netdev", "user,id=net0",
			"-device", "virtio-net-pci,netdev=net0",
		)

		if cfg.DiskPath != "" {
			args = append(args,
				"-drive", fmt.Sprintf("file=%s,format=%s,if=none,id=hd0", cfg.DiskPath, diskFormat),
				"-device", "virtio-blk-pci,drive=hd0",
			)
		}
		if cfg.ISOPath != "" {
			args = append(args,
				"-drive", fmt.Sprintf("file=%s,media=cdrom,if=none,id=cdrom", cfg.ISOPath),
				"-device", "virtio-blk-pci,drive=cdrom",
				"-boot", "d",
			)
		}
	} else {
		args = append(args, "-M", "q35")
		switch {
		case cfg.EnableAccel && isDarwinX86:
			args = append(args, "-accel", "hvf", "-cpu", "host")
		case cfg.EnableAccel && isLinuxX86:
			args = append(args, "-accel", "kvm", "-cpu", "host")
		default:
			args = append(args, "-accel", "tcg,thread=multi,tb-size=512", "-cpu", "Nehalem")
		}

		if runtime.GOOS == "darwin" {
			args = append(args, "-display", "cocoa")
		}
		args = append(args,
			"-vga", "virtio",
			"-usb", "-device", "usb-tablet",
			"-netdev", "user,id=net0",
			"-device", "virtio-net-pci,netdev=net0",
		)

		if cfg.DiskPath != "" {
			args = append(args, "-drive", fmt.Sprintf("file=%s,format=%s,if=virtio", cfg.DiskPath, diskFormat))
		}
		if cfg.ISOPath != "" {
			args = append(args, "-cdrom", cfg.ISOPath, "-boot", "d")
		}
	}

	if strings.TrimSpace(cfg.ExtraArgs) != "" {
		args = append(args, strings.Fields(cfg.ExtraArgs)...)
	}

	return bin, args, nil
}

// StartVM launches a QEMU VM process and streams its stdout/stderr logs.
func (m *Manager) StartVM(cfg VMConfig) (*VMState, error) {
	m.mu.Lock()
	if p, ok := m.processes[cfg.ID]; ok && p.Cmd != nil && p.Cmd.Process != nil {
		pid := p.Cmd.Process.Pid
		m.mu.Unlock()
		return nil, fmt.Errorf("a máquina virtual %q já está em execução (PID %d)", cfg.Name, pid)
	}
	m.mu.Unlock()

	if err := m.EnsureDisk(&cfg); err != nil {
		return nil, err
	}

	bin, args, err := m.BuildCommand(cfg)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(bin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar stdout do QEMU: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar stderr do QEMU: %w", err)
	}

	if err := cmd.Start(); err != nil {
		state := VMState{
			ID:      cfg.ID,
			Status:  "error",
			LastLog: err.Error(),
		}
		m.mu.Lock()
		m.states[cfg.ID] = state
		m.mu.Unlock()
		m.emitEvent("vms:status", state)
		return nil, fmt.Errorf("falha ao iniciar processo do QEMU: %w", err)
	}

	handle := &ProcessHandle{
		Config:    cfg,
		Cmd:       cmd,
		StartedAt: time.Now(),
		Cancel:    make(chan struct{}),
		Logs:      make([]string, 0, 100),
	}

	state := VMState{
		ID:        cfg.ID,
		Status:    "running",
		PID:       cmd.Process.Pid,
		StartedAt: handle.StartedAt.Format(time.RFC3339),
	}

	m.mu.Lock()
	m.processes[cfg.ID] = handle
	m.states[cfg.ID] = state
	m.mu.Unlock()

	m.emitEvent("vms:status", state)

	go m.consumeLogStream(cfg.ID, stdout)
	go m.consumeLogStream(cfg.ID, stderr)
	go m.monitorProcess(cfg.ID, cmd)

	return &state, nil
}

func (m *Manager) monitorProcess(id string, cmd *exec.Cmd) {
	waitErr := cmd.Wait()

	m.mu.Lock()
	delete(m.processes, id)

	currentState := m.states[id]
	if waitErr != nil {
		currentState.Status = "error"
		currentState.LastLog = fmt.Sprintf("Processo finalizado com erro: %v", waitErr)
	} else {
		currentState.Status = "stopped"
	}
	currentState.PID = 0
	m.states[id] = currentState
	m.mu.Unlock()

	m.emitEvent("vms:status", currentState)
}

// StopVM terminates a running VM via SIGINT/SIGTERM or SIGKILL when force is true.
func (m *Manager) StopVM(id string, force bool) error {
	m.mu.RLock()
	p, ok := m.processes[id]
	m.mu.RUnlock()

	if !ok || p.Cmd == nil || p.Cmd.Process == nil {
		return fmt.Errorf("máquina virtual %s não está em execução", id)
	}

	if force {
		return p.Cmd.Process.Kill()
	}
	return p.Cmd.Process.Signal(os.Interrupt)
}

// StopAll forcefully terminates any running VMs during application shutdown.
func (m *Manager) StopAll() {
	m.mu.RLock()
	cmds := make([]*exec.Cmd, 0, len(m.processes))
	for _, p := range m.processes {
		if p.Cmd != nil && p.Cmd.Process != nil {
			cmds = append(cmds, p.Cmd)
		}
	}
	m.mu.RUnlock()

	for _, cmd := range cmds {
		_ = cmd.Process.Signal(os.Interrupt)
	}
}

// GetLogs returns a snapshot copy of buffered output lines for a VM.
func (m *Manager) GetLogs(id string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if p, ok := m.processes[id]; ok {
		return slices.Clone(p.Logs)
	}
	return nil
}

// GetAllStates returns runtime states with up-to-date uptime for all tracked VMs.
func (m *Manager) GetAllStates() map[string]VMState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make(map[string]VMState, len(m.states))
	for k, v := range m.states {
		if p, ok := m.processes[k]; ok && v.Status == "running" {
			v.UptimeSec = int(time.Since(p.StartedAt).Seconds())
		}
		res[k] = v
	}
	return res
}

func (m *Manager) consumeLogStream(id string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		text := scanner.Text()
		m.mu.Lock()
		if p, ok := m.processes[id]; ok {
			if len(p.Logs) > 300 {
				p.Logs = p.Logs[len(p.Logs)-200:]
			}
			p.Logs = append(p.Logs, text)
		}
		m.mu.Unlock()

		m.emitEvent("vms:log", map[string]any{
			"id":   id,
			"line": text,
			"time": time.Now().Format("15:04:05"),
		})
	}
}

func (m *Manager) emitEvent(event string, data any) {
	if m.onEvent != nil {
		m.onEvent(event, data)
	}
}

func sanitizeFileName(name string) string {
	clean := strings.ToLower(strings.TrimSpace(name))
	clean = strings.ReplaceAll(clean, " ", "_")
	var b strings.Builder
	for _, r := range clean {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "vm"
	}
	return b.String()
}
