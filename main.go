// Custom Vencord Installer.
//
// One-click first apply of the custom Vencord build, for someone who
// already has regular Vencord installed on Discord desktop (Windows).
//
// Regular Vencord already patched Discord to load Vencord from its user
// data folder, so this installer does not re-implement Discord patching
// at all: it reuses that patch, backs up the existing dist folder,
// downloads the builder repo's latest release into it, enables the two
// custom plugins in settings.json, and launches Discord. After that,
// the CustomUpdater plugin in the build handles all future updates.
//
// If regular Vencord is not installed (no dist/patcher.js), this exits
// with instructions instead of patching Discord itself. Install regular
// Vencord once with the official installer, then run this again.
//
// Stdlib only, no external dependencies.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var distFiles = []string{"patcher.js", "preload.js", "renderer.js", "renderer.css"}

var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Name    string         `json:"name"`
	Assets  []releaseAsset `json:"assets"`
}

type versionFile struct {
	Version string `json:"version"`
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func defaultDistDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA is not set")
		}
		return filepath.Join(appData, "Vencord", "dist"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", "Vencord", "dist"), nil
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", "Vencord", "dist"), nil
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func fetchJSON(url string, target any) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "CustomVencordInstaller")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(target)
}

func downloadBytes(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "CustomVencordInstaller")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, res.Status)
	}
	return io.ReadAll(res.Body)
}

func discordRunning() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	for _, name := range []string{"Discord.exe", "DiscordPTB.exe", "DiscordCanary.exe"} {
		out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq "+name, "/NH").Output()
		if err == nil && strings.Contains(string(out), name) {
			return true
		}
	}
	return false
}

func waitForDiscordQuit() {
	reader := bufio.NewReader(os.Stdin)
	for discordRunning() {
		fmt.Println("Discord is still running. Quit Discord (tray icon > Quit Discord), then press Enter.")
		_, _ = reader.ReadString('\n')
	}
}

func launchDiscord() {
	if runtime.GOOS != "windows" {
		fmt.Println("Not launching Discord automatically on this OS; launch it yourself.")
		return
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		updateExe := filepath.Join(localAppData, "Discord", "Update.exe")
		if _, err := os.Stat(updateExe); err == nil {
			cmd := exec.Command(updateExe, "--processStart", "Discord.exe")
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Start(); err == nil {
				return
			}
		}
	}
	// Fallback: launch the newest app-* Discord.exe directly.
	matches, _ := filepath.Glob(filepath.Join(localAppData, "Discord", "app-*", "Discord.exe"))
	if len(matches) > 0 {
		cmd := exec.Command(matches[len(matches)-1])
		if err := cmd.Start(); err == nil {
			return
		}
	}
	fmt.Println("Could not find Discord to launch automatically; launch it yourself.")
}

func enablePlugins(settingsPath, version string) error {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return err
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("settings.json is not valid JSON: %w", err)
	}
	plugins, ok := settings["plugins"].(map[string]any)
	if !ok {
		plugins = map[string]any{}
		settings["plugins"] = plugins
	}
	setPlugin := func(name string, fields map[string]any) {
		entry, ok := plugins[name].(map[string]any)
		if !ok {
			entry = map[string]any{}
			plugins[name] = entry
		}
		for k, v := range fields {
			entry[k] = v
		}
	}
	setPlugin("CustomStream-Preview", map[string]any{"enabled": true})
	setPlugin("CustomUpdater", map[string]any{"enabled": true, "appliedVersion": version})
	setPlugin("VoiceSpectrum", map[string]any{"enabled": true})

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return err
	}
	return os.WriteFile(settingsPath, buf.Bytes(), 0o644)
}

func printRecommendedPath() {
	fmt.Println("Recommended path (this installer does not patch Discord itself):")
	fmt.Println("  1. Install regular Vencord with the official Vencord installer.")
	fmt.Println("  2. Launch Discord once and check Vencord settings appear, so you know the patch works.")
	fmt.Println("  3. Quit Discord, then run this installer again. It switches that install to the custom build.")
	fmt.Println("Fresh install and repair install are the same path: official installer first, this installer second.")
	fmt.Println("If Vencord disappeared after a Discord update, that is a repair: run the official installer again,")
	fmt.Println("verify regular Vencord works, then run this installer again. Official install/repair does not")
	fmt.Println("wipe your Vencord settings, themes, or QuickCSS; they live outside the dist folder.")
}

func main() {
	flag.Usage = func() {
		fmt.Println("Custom Vencord Installer: switches an existing regular Vencord install to the custom build.")
		fmt.Println()
		printRecommendedPath()
		fmt.Println()
		fmt.Println("Flags:")
		flag.PrintDefaults()
	}
	repo := flag.String("repo", "MLG-SERBUR/vencord-customstream-build", "Builder repo, as owner/name")
	distFlag := flag.String("dist", "", "Vencord dist folder (default: OS default)")
	settingsFlag := flag.String("settings", "", "Vencord settings.json (default: <dist parent>/settings/settings.json)")
	noLaunch := flag.Bool("no-launch", false, "Do not launch Discord after installing")
	yes := flag.Bool("yes", false, "Skip the confirmation prompt")
	restore := flag.String("restore", "", "Restore dist from this backup folder instead of downloading, then exit")
	flag.Parse()

	if !repoRe.MatchString(*repo) {
		fail(fmt.Errorf("invalid repo: %q, expected owner/name", *repo))
	}

	distDir := *distFlag
	if distDir == "" {
		var err error
		distDir, err = defaultDistDir()
		if err != nil {
			fail(err)
		}
	}
	settingsPath := *settingsFlag
	if settingsPath == "" {
		settingsPath = filepath.Join(filepath.Dir(distDir), "settings", "settings.json")
	}

	// v1 scope gate: regular Vencord must already be installed, so
	// Discord is already patched to load from this dist folder.
	if _, err := os.Stat(filepath.Join(distDir, "patcher.js")); err != nil {
		fmt.Printf("Vencord not installed (or Discord patch missing): no patcher.js at %s.\n", distDir)
		fmt.Println("This can mean a fresh install with no Vencord yet, Vencord uninstalled,")
		fmt.Println("or a Discord update that removed the Vencord patch. In all three cases")
		fmt.Println("the fix is the same, and running this installer alone will not fix it,")
		fmt.Println("because this installer only replaces dist files; it does not patch Discord.")
		fmt.Println()
		printRecommendedPath()
		os.Exit(1)
	}
	fmt.Printf("Found regular Vencord at %s; switching it to the custom build.\n", distDir)

	waitForDiscordQuit()

	if *restore != "" {
		if err := copyDir(*restore, distDir); err != nil {
			fail(err)
		}
		fmt.Printf("Restored dist from %s.\n", *restore)
		if !*noLaunch {
			launchDiscord()
		}
		return
	}

	if !*yes {
		fmt.Printf("Install custom Vencord build from %s\ninto %s? [y/N] ", *repo, distDir)
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(line)) != "y" {
			fmt.Println("Aborted.")
			return
		}
	}

	// Back up first, so a failed download or write can be undone.
	stamp := time.Now().Format("20060102-150405")
	backupDir := distDir + "-backup-" + stamp
	if err := copyDir(distDir, backupDir); err != nil {
		fail(fmt.Errorf("backing up dist: %w", err))
	}
	fmt.Printf("Backed up dist to %s\n", backupDir)
	if data, err := os.ReadFile(settingsPath); err == nil {
		_ = os.WriteFile(settingsPath+".backup-"+stamp, data, 0o644)
	}

	// Download everything before writing anything, so a failed
	// download leaves the existing install untouched.
	var rel release
	if err := fetchJSON("https://api.github.com/repos/"+*repo+"/releases/latest", &rel); err != nil {
		fail(fmt.Errorf("fetching latest release: %w", err))
	}
	assets := map[string]string{}
	for _, a := range rel.Assets {
		assets[a.Name] = a.BrowserDownloadURL
	}
	versionURL, ok := assets["version.json"]
	if !ok {
		fail(fmt.Errorf("latest release has no version.json"))
	}
	versionData, err := downloadBytes(versionURL)
	if err != nil {
		fail(fmt.Errorf("downloading version.json: %w", err))
	}
	var ver versionFile
	if err := json.Unmarshal(versionData, &ver); err != nil || ver.Version == "" {
		fail(fmt.Errorf("version.json has no version"))
	}
	downloads := map[string][]byte{"version.json": versionData}
	for _, name := range distFiles {
		url, ok := assets[name]
		if !ok {
			fail(fmt.Errorf("latest release is missing %s", name))
		}
		if !strings.HasPrefix(url, "https://github.com/"+*repo+"/releases/download/") {
			fail(fmt.Errorf("refusing download from unexpected URL for %s", name))
		}
		data, err := downloadBytes(url)
		if err != nil {
			fail(fmt.Errorf("downloading %s: %w", name, err))
		}
		downloads[name] = data
	}

	for name, data := range downloads {
		if err := os.WriteFile(filepath.Join(distDir, name), data, 0o644); err != nil {
			// Best effort restore, then report the original error.
			_ = copyDir(backupDir, distDir)
			fail(fmt.Errorf("writing %s: %w", name, err))
		}
	}
	fmt.Printf("Installed custom Vencord build %s\n", ver.Version)

	if err := enablePlugins(settingsPath, ver.Version); err != nil {
		fmt.Printf("Warning: could not update settings.json (%v). Enable CustomStream-Preview, CustomUpdater, and VoiceSpectrum manually in Vencord settings > Plugins.\n", err)
	} else {
		fmt.Println("Enabled CustomStream-Preview, CustomUpdater, and VoiceSpectrum, set appliedVersion so the updater does not re-download this build.")
	}

	if !*noLaunch {
		launchDiscord()
	}
	fmt.Println("Done. Future updates arrive via CustomUpdater on Discord startup: it applies them and restarts automatically.")
}
