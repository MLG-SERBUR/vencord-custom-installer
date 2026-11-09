# Custom Vencord Installer

Standalone installer for the custom Vencord build, in the style of the
official Vencord installer, but for this build only.

Download `CustomVencordInstaller.exe` from the `latest` release in this
repo, double-click it, confirm, and it does the first apply for you.

## Before you run this: official installer first, then this installer

This is the recommended path, for fresh installs and for repairs, and
it is also what the installer itself prints when it is run without
Vencord installed:

1. Install regular Vencord with the official Vencord installer.
2. Launch Discord once and check Vencord settings appear, so you know
   the patch works.
3. Quit Discord, then run this installer. It switches that install to
   the custom build.

Fresh install and repair install are the same path: official installer
first, this installer second. If Vencord disappeared after a Discord
update, that is a repair: run the official installer again, verify
regular Vencord works, then run this installer again. Do not run this
installer first on a fresh Discord: it checks for `dist/patcher.js`,
finds nothing, and exits with the steps above. That is by design, not
a bug — this installer only replaces dist files, it does not patch
Discord. Official install/repair does not wipe your Vencord settings,
themes, or QuickCSS; they live outside the dist folder.

If only the custom build files are broken but the Discord patch is
intact (Vencord still loads, just the custom build misbehaves), you do
not need the official step: just rerun this installer, or use
`--restore <backup folder>` to go back to regular Vencord.

## Why a separate repo, not the updater repo

The updater repo's root *is* the plugin folder: the builder clones it
whole into `src/userplugins/customUpdater.desktop`, and Vencord builds
whatever is in that folder as a plugin. Go code, a workflow, and release
assets do not belong in a plugin folder.

This follows the official precedent too: Vencord and its installer are
separate repos (Vendicated/Vencord vs Vendicated/VencordInstaller).
Same split here: the builder repo is the source of builds, this repo is
the program that installs them, the updater plugin is the program that
keeps them updated afterwards.

## What it does (Windows, Discord desktop)

1. Checks regular Vencord is already installed: `dist/patcher.js` must
   exist in `%APPDATA%\Vencord\dist`. That existing install already
   patched Discord to load Vencord from that folder, so this installer
   does **not** patch Discord itself. If that file is missing, it stops
   and tells you to install regular Vencord once with the official
   installer, then run this again. Patching Discord from scratch is
   deliberately out of scope for v1; reusing the official patch is
   safer than reinventing asar patching.
2. Waits for Discord to quit (tray icon > Quit Discord) if it is running.
3. Backs up the dist folder to `dist-backup-<timestamp>`, and
   `settings.json` to `settings.json.backup-<timestamp>`.
4. Downloads the builder repo's `latest` release
   (`MLG-SERBUR/vencord-customstream-build` by default): `version.json`
   first, then `patcher.js`, `preload.js`, `renderer.js`, `renderer.css`.
   Everything is downloaded before anything is written, so a failed
   download leaves the existing install untouched. A failed write
   restores the backup.
5. Edits `settings.json` to enable `CustomStream-Preview`,
   `CustomUpdater`, and `VoiceSpectrum`, and sets `CustomUpdater.appliedVersion` to the
   installed version, so the updater does not re-download this build on
   first launch. Other settings are left as-is (the file is rewritten
   with sorted keys; the backup from step 3 covers that).
6. Launches Discord via `Update.exe --processStart Discord.exe`, unless
   `--no-launch` is given.

After that, CustomUpdater handles future updates on Discord startup:
it applies them and restarts automatically, with no notification.

## Usage

```
CustomVencordInstaller.exe
CustomVencordInstaller.exe --yes --no-launch
CustomVencordInstaller.exe --repo MLG-SERBUR/vencord-customstream-build
CustomVencordInstaller.exe --restore "%APPDATA%\Vencord\dist-backup-20261031-120000"
```

Flags: `--repo`, `--dist`, `--settings`, `--no-launch`, `--yes`,
`--restore`. Run with `-h` for the full list.

## Revert

Quit Discord, then either run the installer with `--restore <backup
folder>`, or copy the backup folder back over `dist` manually and
launch Discord. You are back on regular Vencord (whose stock updater
will then update to official again, as before).

## Building

Stdlib only, no external Go dependencies.

```sh
GOOS=windows GOARCH=amd64 go build -o CustomVencordInstaller.exe .
```

Releases are built by `.github/workflows/release.yml` on every push to
`main` (rolling `latest` release) and on manual dispatch.
