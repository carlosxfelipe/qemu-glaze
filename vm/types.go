package vm

import (
	"fmt"
	"strings"
)

// VMConfig defines the user configuration for a virtual machine.
type VMConfig struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Arch        string `json:"arch"`
	Cores       int    `json:"cores"`
	MemoryMB    int    `json:"memoryMB"`
	ISOPath     string `json:"isoPath"`
	DiskPath    string `json:"diskPath"`
	DiskSizeGB  int    `json:"diskSizeGB"`
	CreateDisk  bool   `json:"createDisk"`
	EnableAccel bool   `json:"enableAccel"`
	ExtraArgs   string `json:"extraArgs"`
	CreatedAt   string `json:"createdAt"`
}

// Validate checks whether the VM configuration meets required bounds and fields.
func (c *VMConfig) Validate() error {
	name := strings.TrimSpace(c.Name)
	if len([]rune(name)) < 2 || len([]rune(name)) > 50 {
		return fmt.Errorf("o nome da máquina deve ter entre 2 e 50 caracteres")
	}
	if c.Arch != "x86_64" && c.Arch != "aarch64" {
		return fmt.Errorf("arquitetura inválida %q (use x86_64 ou aarch64)", c.Arch)
	}
	if c.Cores < 1 || c.Cores > 64 {
		return fmt.Errorf("número de núcleos inválido (%d): deve ficar entre 1 e 64", c.Cores)
	}
	if c.MemoryMB < 256 || c.MemoryMB > 131072 {
		return fmt.Errorf("memória RAM inválida (%d MB): deve ficar entre 256 MB e 131072 MB", c.MemoryMB)
	}
	return nil
}

// VMState represents runtime process information for a virtual machine.
type VMState struct {
	ID        string `json:"id"`
	Status    string `json:"status"` // "stopped", "starting", "running", "error"
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt,omitempty"`
	UptimeSec int    `json:"uptimeSec"`
	LastLog   string `json:"lastLog,omitempty"`
}

// SystemInfo represents host system capabilities and QEMU detection.
type SystemInfo struct {
	OS              string   `json:"os"`
	Arch            string   `json:"arch"`
	HostCPUs        int      `json:"hostCpus"`
	HostMemoryMB    int      `json:"hostMemoryMB"`
	HostDiskFreeGB  int      `json:"hostDiskFreeGB"`
	HostDiskTotalGB int      `json:"hostDiskTotalGB"`
	QemuVersion     string   `json:"qemuVersion"`
	QemuX86Path     string   `json:"qemuX86Path"`
	QemuArmPath     string   `json:"qemuArmPath"`
	QemuImgPath     string   `json:"qemuImgPath"`
	Accelerators    []string `json:"accelerators"`
	Edk2ArmPath     string   `json:"edk2ArmPath"`
	Edk2X86Path     string   `json:"edk2X86Path"`
	DisksDir        string   `json:"disksDir"`
}

// SaveVMRequest wraps a VM configuration for persistence.
type SaveVMRequest struct {
	VM VMConfig `json:"vm"`
}

// StartVMRequest identifies either a saved VM by ID or an ad-hoc Quick Boot config.
type StartVMRequest struct {
	ID    string    `json:"id"`
	AdHoc *VMConfig `json:"adHoc,omitempty"`
}

// StopVMRequest identifies a VM to stop or query logs for.
type StopVMRequest struct {
	ID    string `json:"id"`
	Force bool   `json:"force"`
}

// DeleteVMRequest identifies a VM to delete and whether to remove its disk.
type DeleteVMRequest struct {
	ID         string `json:"id"`
	DeleteDisk bool   `json:"deleteDisk"`
}

// CreateDiskRequest configures creation of a new QCOW2 virtual disk.
type CreateDiskRequest struct {
	Name   string `json:"name"`
	SizeGB int    `json:"sizeGB"`
}

// CreateDiskResponse returns the created disk path and size.
type CreateDiskResponse struct {
	Path   string `json:"path"`
	SizeGB int    `json:"sizeGB"`
}

// BuildCommandResponse describes the generated QEMU command preview.
type BuildCommandResponse struct {
	Binary  string   `json:"binary"`
	Args    []string `json:"args"`
	Command string   `json:"command"`
}

// VMListResponse returns saved VMs alongside their current runtime states.
type VMListResponse struct {
	Items  []VMConfig         `json:"items"`
	States map[string]VMState `json:"states"`
}

// LogResponse returns buffered console output lines for a VM.
type LogResponse struct {
	Lines []string `json:"lines"`
}

// BrowseResponse returns a selected filesystem path from a native file dialog.
type BrowseResponse struct {
	Path string `json:"path"`
}
