//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	keepAppDir := flag.Bool("keep-appdir", false, "Keep temporary AppDir directory after packaging")
	flag.Parse()

	appName := "QEMU Glaze"
	execName := "qemu-glaze"
	version := "1.0.0"
	description := "Gerenciador de Máquinas Virtuais QEMU com Glaze (CGO_ENABLED=0)."
	categories := "System;Emulator;Utility;"

	arch := appImageArch(runtime.GOARCH)
	if arch == "" {
		fatalf("unsupported architecture for AppImage: %s", runtime.GOARCH)
	}

	appDirName := strings.ReplaceAll(appName, " ", "_") + ".AppDir"
	outputAppImage := fmt.Sprintf("%s-%s-%s.AppImage", strings.ReplaceAll(appName, " ", "_"), version, arch)

	fmt.Printf("📦 Building Linux AppDir/AppImage: %s\n", outputAppImage)

	_ = os.RemoveAll(appDirName)
	_ = os.Remove(outputAppImage)

	binDir := filepath.Join(appDirName, "usr", "bin")
	applicationsDir := filepath.Join(appDirName, "usr", "share", "applications")
	iconsDir := filepath.Join(appDirName, "usr", "share", "icons", "hicolor", "256x256", "apps")

	for _, d := range []string{binDir, applicationsDir, iconsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			fatalf("failed to create %s: %v", d, err)
		}
	}

	binPath := filepath.Join(binDir, execName)
	fmt.Printf("🔨 Compiling Linux binary (CGO_ENABLED=0) -> %s\n", binPath)

	buildCmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", binPath, ".")
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fatalf("go build failed: %s (%v)", string(out), err)
	}

	iconSrc := filepath.Join("assets", "icon.png")
	if data, err := os.ReadFile(iconSrc); err == nil {
		_ = os.WriteFile(filepath.Join(appDirName, execName+".png"), data, 0644)
		_ = os.WriteFile(filepath.Join(appDirName, ".DirIcon"), data, 0644)
		_ = os.WriteFile(filepath.Join(iconsDir, execName+".png"), data, 0644)
	}

	desktopContent := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Exec=%s %%u
Icon=%s
Comment=%s
Categories=%s
Terminal=false
StartupWMClass=%s
`, appName, execName, execName, description, categories, execName)

	_ = os.WriteFile(filepath.Join(appDirName, execName+".desktop"), []byte(desktopContent), 0644)
	_ = os.WriteFile(filepath.Join(applicationsDir, execName+".desktop"), []byte(desktopContent), 0644)

	appRunContent := fmt.Sprintf(`#!/bin/sh
SELF=$(readlink -f "$0")
HERE=${SELF%%/*}
export PATH="${HERE}/usr/bin:${PATH}"
export LD_LIBRARY_PATH="${HERE}/usr/lib:${LD_LIBRARY_PATH}"
exec "${HERE}/usr/bin/%s" "$@"
`, execName)
	_ = os.WriteFile(filepath.Join(appDirName, "AppRun"), []byte(appRunContent), 0755)

	toolPath, err := exec.LookPath("appimagetool")
	if err != nil {
		fmt.Printf("ℹ️  appimagetool não encontrado no PATH; AppDir gerado em %s\n", appDirName)
		return
	}

	pkgCmd := exec.Command(toolPath, appDirName, outputAppImage)
	pkgCmd.Env = append(os.Environ(),
		fmt.Sprintf("ARCH=%s", arch),
		"APPIMAGE_EXTRACT_AND_RUN=1",
	)
	pkgCmd.Stdout = os.Stdout
	pkgCmd.Stderr = os.Stderr
	if err := pkgCmd.Run(); err != nil {
		fatalf("appimagetool failed: %v", err)
	}

	if !*keepAppDir {
		_ = os.RemoveAll(appDirName)
	}
	fmt.Printf("\n✨ Successfully generated: %s\n", outputAppImage)
}

func appImageArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return ""
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
