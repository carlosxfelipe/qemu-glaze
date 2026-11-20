//go:build ignore

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	appName := "QEMU Glaze"
	bundleID := "com.carlosxfelipe.qemu-glaze"
	version := "1.0.0"

	appBundle := appName + ".app"
	contentsDir := filepath.Join(appBundle, "Contents")
	macOSDir := filepath.Join(contentsDir, "MacOS")
	resourcesDir := filepath.Join(contentsDir, "Resources")

	fmt.Printf("📦 Building macOS Application Bundle: %s\n", appBundle)

	_ = os.RemoveAll(appBundle)
	if err := os.MkdirAll(macOSDir, 0755); err != nil {
		fatalf("failed to create MacOS directory: %v", err)
	}
	if err := os.MkdirAll(resourcesDir, 0755); err != nil {
		fatalf("failed to create Resources directory: %v", err)
	}

	binPath := filepath.Join(macOSDir, appName)
	fmt.Printf("🔨 Compiling pure-Go macOS binary (CGO_ENABLED=0) -> %s\n", binPath)

	buildCmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", binPath, ".")
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		fatalf("go build failed: %s (%v)", string(out), err)
	}

	icnsSrc := filepath.Join("assets", "app.icns")
	icnsDst := filepath.Join(resourcesDir, "app.icns")
	if data, err := os.ReadFile(icnsSrc); err == nil {
		if err := os.WriteFile(icnsDst, data, 0644); err != nil {
			fatalf("failed to write app.icns: %v", err)
		}
		fmt.Printf("🎨 Copied icon: %s -> %s\n", icnsSrc, icnsDst)
	}

	plistPath := filepath.Join(contentsDir, "Info.plist")
	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>%s</string>
    <key>CFBundleDisplayName</key>
    <string>%s</string>
    <key>CFBundleIdentifier</key>
    <string>%s</string>
    <key>CFBundleVersion</key>
    <string>%s</string>
    <key>CFBundleShortVersionString</key>
    <string>%s</string>
    <key>CFBundleExecutable</key>
    <string>%s</string>
    <key>CFBundleIconFile</key>
    <string>app.icns</string>
    <key>LSMinimumSystemVersion</key>
    <string>11.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
`, appName, appName, bundleID, version, version, appName)

	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		fatalf("failed to write Info.plist: %v", err)
	}

	_ = os.WriteFile(filepath.Join(contentsDir, "PkgInfo"), []byte("APPL????"), 0644)

	fmt.Printf("\n✨ Successfully generated: %s\n", appBundle)
	fmt.Printf("👉 To run: open %q\n", appBundle)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
