<img src="assets/icon.png" alt="" width="96" align="right">

# ProDJ Tools

[Русский](README.md) · **English**

[![Download](https://img.shields.io/github/v/release/EgaNet1/ProDJ-Tools?label=download&color=f0a330)](https://github.com/EgaNet1/ProDJ-Tools/releases/latest)
[![Build](https://github.com/EgaNet1/ProDJ-Tools/actions/workflows/build.yml/badge.svg)](https://github.com/EgaNet1/ProDJ-Tools/actions/workflows/build.yml)
[![Windows 10/11](https://img.shields.io/badge/Windows-10%20%7C%2011-0078d4)](#installation)
[![MIT license](https://img.shields.io/badge/license-MIT-green)](LICENSE)

A desktop app that turns a folder of tracks into a DJ set. It detects the tempo, key and energy of every track, orders the tracks so they mix smoothly, and writes that order into the files themselves, so Rekordbox, Serato and any player can see it without this app.

The interface speaks 15 languages: English, Español, Português, Français, Deutsch, Italiano, Polski, Русский, Türkçe, العربية, हिन्दी, Bahasa Indonesia, 中文, 日本語, 한국어.

## Installation

### ⬇️ [Download the installer, ProDJTools-Setup.exe](https://github.com/EgaNet1/ProDJ-Tools/releases/latest)

1. On the latest release page, download `ProDJTools-Setup-<version>.exe` from **Assets**.
2. Run it. No administrator rights are needed: the app is installed for the current user in `%LOCALAPPDATA%\Programs\ProDJ Tools`, with a Start menu shortcut and an optional desktop shortcut. The installer is available in English and Russian.
3. The installer is not code-signed, so Windows SmartScreen may say "Windows protected your PC". Click **More info → Run anyway**.

No installation: the same page has a portable `ProDJTools.exe`. Put it in any folder and run it.

A new version installs over the old one and keeps your settings and analysis cache. To uninstall, go to Settings → Apps. The uninstaller asks whether to remove the cache and settings, and it never touches your music folders.

**Requirements:** Windows 10 or 11 (64-bit). The app window is opened with Microsoft Edge, which comes with Windows, or with Chrome.

## Running

Start ProDJ Tools from the Start menu. It opens in its own window (Edge or Chrome in app mode). The app exits when the window is closed.

Supported formats: FLAC, MP3 and WAV. Tags are written to FLAC and MP3.

**Language.** On first start the app follows the language of Windows; change it with the globe button in the top-right corner. The switch is instant, with no restart, and covers the interface, the transition advice, the cue sheet and the export log.

## Features

**Set**
- Analyses a folder on all CPU cores. Results are cached, so opening the folder again takes a fraction of a second.
- Drag tracks to reorder them. A lock keeps a track in place when the set is rebuilt; "−" removes a track from the set.
- Each transition is rated: the tempo change in percent and key compatibility (green, yellow or red).
- "Listen to the joint" plays the end of one track and the start of the next, tempo-matched, with a smooth crossfade.

**Mixing**: cue points for set preparation, like professional DJs use
- A beat grid for every track: tempo to a thousandth of a BPM, first downbeat to a fraction of a millisecond. Grid accuracy is shown (typically ±0.3–2 ms).
- Bar-level structure (intro, drops, breaks, outro) with cues on their downbeats: A MIX IN, B DROP, C BREAK, D DROP 2, E MIX OUT.
- A phrase-aligned plan for each transition: where in track A to start track B, where to swap the bass (B's drop lands on a phrase boundary of A) and when A is fully out, with min:sec.ms times and bar numbers.
- "Preview" plays the transition exactly as planned: B starts on the bar, tempo-matched, and the bass swaps on the downbeat.
- The cue sheet is saved to a text file. On export, the grid and cues go into Rekordbox XML (hot cues and memory cues).

**Tools**
- **Energy curve**: the shape of the set, such as ramp-up, arc, waves, plateau or cool-down. Both energy and tempo follow the curve.
- **Priorities**: what matters most when building, whether that is harmonic mixing, smooth tempo or following the curve.
- **Timed set**: picks the best tracks from the folder to fill a slot of a given length, accounting for mix overlaps.
- **What to play next**: the best follow-ups for the selected track, plus a Camelot wheel with every key in the folder.
- **Transition check**: a list of the transitions that need attention, with hints.
- **Tempo octave**: tells the analyser your genre's tempo range so 70 and 140 BPM are not confused.

**Export**
- File numbering ("07 - Artist - Title.flac"), track numbers in tags, M3U8 playlists and Rekordbox XML.
- Before the first write, the original names and tags are saved to `.prodj-backup.json`. The "Restore files" button undoes all changes.

**Player**: the volume control in the bottom bar applies both to single tracks and to transition previews, and the level is remembered between sessions.

Keys: `Space` pause, `↑` `↓` select a track, `Enter` play, `L` lock, `Del` remove from the set, `M` mute and unmute.

## Building from source

All you need is Windows 10/11, PowerShell and git. You don't have to install Go: on first run, the scripts download the version listed in `go.mod` into the project's `.goenv` folder and verify its checksum. It works like a Python virtual environment, so nothing is installed system-wide.

```powershell
git clone https://github.com/EgaNet1/ProDJ-Tools.git
cd ProDJ-Tools
.\build.ps1                          # vet, test and build ProDJTools.exe
.\build-installer.ps1                # same, plus the installer dist\ProDJTools-Setup-<version>.exe
.\build-installer.ps1 -Version 1.2.0 # explicit version (default: the VERSION file)
```

If Windows refuses to run the scripts, run `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned` once.

The version number lives in [VERSION](VERSION) and goes into the exe properties, the installer and the app header next to the logo. The icon is in [assets/](assets/); `go run assets/genicon.go` draws it.

The installer is built with [Inno Setup 6](https://jrsoftware.org/isinfo.php) from [installer/ProDJTools.iss](installer/ProDJTools.iss). If Inno Setup is not installed, `build-installer.ps1` downloads it into the project's `.tools` folder and verifies its digital signature.

For working on the code directly:

```powershell
. .\activate.ps1       # put Go from .goenv on PATH in this PowerShell window
go test ./...
go run ./cmd/prodj -no-window -port 8080   # server without a window, UI at http://127.0.0.1:8080
deactivate
```

### Releasing

GitHub Actions ([.github/workflows/build.yml](.github/workflows/build.yml)) checks every push. To publish a release, put the new version into `VERSION`, commit, and push a tag with the same number:

```powershell
git tag v1.0.1
git push origin main v1.0.1
```

Actions builds the installer and the portable exe and publishes them under [Releases](https://github.com/EgaNet1/ProDJ-Tools/releases). The Download link at the top of this file always points to the latest release.

## How it works

```
cmd/prodj/            entry point: server and window
internal/
  audio/              FLAC/MP3/WAV decoding straight to ~11 kHz mono
  analysis/           tempo (spectral-flux autocorrelation), beat grid (folding every beat
                      of the track onto one), downbeat, bar structure,
                      key (chromagram and Krumhansl–Kessler profiles), energy
  mixplan/            A–E cues, phrase-aligned transition plans, cue sheet
  camelot/            keys on the Camelot wheel and their compatibility
  library/            parallel folder scan, cache, relative energy
  setlist/            curves, transition scoring, order optimiser, suggestions
  export/             numbering, tags, playlists, backup and restore
  tags/               reading and writing FLAC and MP3 tags
  naming/             numbers in file names and titles
  safefile/           atomic overwrite and rename with retries
  desktop/            app window and the native folder picker
  i18n/               translations: language choice and message lookup
  server/             HTTP API between the interface and the engine
web/                  interface (HTML, CSS, framework-free JS), embedded in the exe
  i18n/               the texts of every language, shared by the interface and the program
assets/               the icon and the script that draws it
installer/            Inno Setup installer script
```

**Ordering the set.** Each possible order gets a cost made up of tempo jumps, key clashes, energy and tempo deviating from the chosen curve, and tempo moving against the curve. The minimum is found by simulated annealing. The app starts from a random order and keeps swapping tracks. It always accepts improvements and sometimes accepts worse orders, less often as time goes on, so it doesn't get stuck on the first acceptable one. The search runs in parallel on all cores and the best result wins. A 30-track set is built in about 30 ms.

**Building the beat grid.** A single hit in a dense mix is hard to pin down, because bass, hi-hats and synths all have attacks. So the whole track is folded onto a single beat: every moment is added to its position within the beat. At the exact tempo, the attacks of hundreds of beats land on one point, while an error of a thousandth of a BPM smears them out. That finds the tempo, and the folded attack (the average of every hit in the track) gives the phase to the sample. The downbeat is found where the sound rises most within the bar, since new parts and drops come in on the one.

**Adding a language.** Copy [web/i18n/en.json](web/i18n/en.json) to `web/i18n/<code>.json`, translate the values (leave the keys and `{0}`, `{1}` as they are) and add the code to `codes` in [internal/i18n/i18n.go](internal/i18n/i18n.go). `go test ./...` checks that every message is translated and the placeholders match.

**Where data is stored.** The analysis cache, settings and log are kept in `%LOCALAPPDATA%\ProDJTools`. The set order for each folder is remembered in the app window.

## Author

[EgaNet1](https://github.com/EgaNet1). Report bugs and ideas in [Issues](https://github.com/EgaNet1/ProDJ-Tools/issues).

## License

[MIT](LICENSE)
