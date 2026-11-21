//go:build ignore

package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
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

	if err := os.WriteFile(filepath.Join(appDirName, execName+".desktop"), []byte(desktopContent), 0644); err != nil {
		fatalf("failed to write root .desktop: %v", err)
	}
	if err := os.WriteFile(filepath.Join(applicationsDir, execName+".desktop"), []byte(desktopContent), 0644); err != nil {
		fatalf("failed to write applications .desktop: %v", err)
	}

	appRunContent := fmt.Sprintf(`#!/bin/sh
SELF=$(readlink -f "$0")
HERE=${SELF%%/*}
export PATH="${HERE}/usr/bin:${PATH}"
export LD_LIBRARY_PATH="${HERE}/usr/lib:${LD_LIBRARY_PATH}"
exec "${HERE}/usr/bin/%s" "$@"
`, execName)
	if err := os.WriteFile(filepath.Join(appDirName, "AppRun"), []byte(appRunContent), 0755); err != nil {
		fatalf("failed to write AppRun: %v", err)
	}

	toolPath, err := ensureAppImageTool(arch)
	if err != nil {
		fatalf("failed to obtain appimagetool: %v", err)
	}

	fmt.Printf("🚀 Packaging AppImage using %s...\n", filepath.Base(toolPath))
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

	fi, err := os.Stat(outputAppImage)
	if err != nil {
		fatalf("failed to verify output AppImage: %v", err)
	}
	sizeMB := float64(fi.Size()) / (1024 * 1024)
	fmt.Printf("\n✨ Successfully generated: %s (%.2f MB)\n", outputAppImage, sizeMB)
	fmt.Printf("👉 To run: ./%s\n", outputAppImage)
}

func appImageArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	case "arm":
		return "armhf"
	default:
		return ""
	}
}

func ensureAppImageTool(arch string) (string, error) {
	if p, err := exec.LookPath("appimagetool"); err == nil {
		return p, nil
	}

	userCache, err := os.UserCacheDir()
	if err != nil {
		userCache = filepath.Join(os.TempDir(), "glaze-cache")
	}
	toolsDir := filepath.Join(userCache, "glaze", "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		return "", err
	}

	toolFileName := fmt.Sprintf("appimagetool-%s.AppImage", arch)
	toolPath := filepath.Join(toolsDir, toolFileName)

	if fi, err := os.Stat(toolPath); err == nil && fi.Size() > 0 {
		return toolPath, nil
	}

	url := fmt.Sprintf("https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-%s.AppImage", arch)
	fmt.Printf("📥 appimagetool não encontrado. Baixando de %s -> %s\n", url, toolPath)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with HTTP status: %s", resp.Status)
	}

	tmpFile := toolPath + ".tmp"
	out, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		_ = os.Remove(tmpFile)
		return "", fmt.Errorf("failed writing downloaded file: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpFile)
		return "", err
	}

	if err := os.Rename(tmpFile, toolPath); err != nil {
		return "", err
	}

	if err := os.Chmod(toolPath, 0755); err != nil {
		return "", err
	}

	return toolPath, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
