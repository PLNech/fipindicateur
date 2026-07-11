# Install & usage

Linux (Ubuntu 24.04, GNOME/X11) is the supported target today. macOS and Arch
are kept buildable and planned.

## Dependencies

Runtime needs **libmpv** (the app links libmpv via cgo, and mpv plays the
streams).

| OS | Runtime | Build |
|----|---------|-------|
| Ubuntu / Debian | `sudo apt install libmpv2` | `sudo apt install libmpv-dev` |
| Arch | `sudo pacman -S mpv` | `sudo pacman -S mpv` |
| macOS | planned | `brew install mpv` |

Go 1.26+ is required (see `go.mod`).

## Install (recommended)

A user-level desktop install, no sudo:

```sh
make install     # builds to ~/.local/bin, adds launcher + icons
make uninstall   # removes everything (binary, launcher, icons, autostart)
```

After `make install` the app appears in GNOME activities (Super, type "fip").
Launching it while it already runs exits the second copy cleanly (single-
instance guard on the MPRIS D-Bus name).

> On GNOME the system-tray (StatusNotifierItem) needs the
> **ubuntu-appindicators** extension enabled; it ships enabled on Ubuntu.

## Build and run in place

```sh
go build -o fipindicateur ./cmd/fipindicateur
./fipindicateur
```

### `go install`

```sh
go install github.com/PLNech/fipindicateur/cmd/fipindicateur@latest
```

This works **only with a local C toolchain**: the app links libmpv via cgo, so
the installing machine needs a C compiler, `pkg-config`, and libmpv's headers
(`mpv.pc`, `mpv/client.h`), the same build prerequisites as the table above
(`libmpv-dev` / `mpv`). Without them the build fails with a pkg-config lookup
error. There is **no `go install` path on Windows** (the Windows build needs a
hand-managed libmpv dev drop; see below).

## Package channels

Every source-build channel (AUR, Homebrew, Nix, `go install`) compiles against
your own libmpv, so no cross-compilation or bundling is involved.

### Debian / Ubuntu (`.deb`)

*Status: available on GitHub Releases from the next tag onward (the packaging
job is wired into CI; the existing v0.1.0-v0.3.0 releases predate it).*

```sh
sudo apt install ./fipindicateur_<version>_amd64.deb   # apt resolves libmpv2
```

The `.deb` declares `Depends: libmpv2`, so `apt` pulls the runtime library from
the standard repos. To build it yourself from a checkout, see
[`packaging/nfpm.yaml`](../packaging/nfpm.yaml).

### Arch (AUR)

*Status: prepared, not yet published (AUR account registration is currently
disabled).* Recipes live in [`packaging/aur/`](../packaging/aur/). Once
published:

```sh
yay -S fipindicateur        # versioned, tracks releases
yay -S fipindicateur-git    # rolling, builds main
```

### macOS / Linux (Homebrew tap)

*Status: formula prepared; the tap repo is not yet created.* See
[`packaging/homebrew/`](../packaging/homebrew/). Once the tap exists:

```sh
brew install PLNech/tap/fipindicateur
```

> macOS is CI-built against `brew install mpv` but has **never been run on real
> macOS hardware**; treat the tray app as unverified there.

### Nix (flake)

*Status: available now (self-serve; needs the repo commit pushed for the
`github:` form).*

```sh
nix run github:PLNech/fipindicateur     # run without installing
nix profile install github:PLNech/fipindicateur
nix develop github:PLNech/fipindicateur # go + libmpv + pkg-config dev shell
```

## Usage

Launch `fipindicateur`. A broadcast-waves glyph appears in your top bar; click
it for the menu. Your last station and settings are remembered in
`~/.config/fipindicateur/config.json`.

- **Launch at login:** toggle *Réglages, Lancer au démarrage* (writes
  `~/.config/autostart/fipindicateur.desktop`).
- **Statistiques:** enable *Réglages, Statistiques d'écoute (local)*, then
  `fipindicateur stats` (or *Voir le rapport*) opens your listening report.
  `fipindicateur stats --out report.html --no-open` just writes the file.
  `fipindicateur version` prints the running build.
- **Mises à jour:** *À propos, Mises à jour, Vérifier maintenant* compares your
  version to the latest GitHub release (picking the asset for your OS) and
  notifies you, opening the release page when a newer one exists. *Vérifier au
  démarrage* (off by default) opts into a quiet check at launch. *Relancer*
  restarts the app to load a freshly installed binary.
- **Media keys / playerctl:**

  ```sh
  playerctl -p fipindicateur play-pause
  playerctl -p fipindicateur metadata
  ```

### Notifications on GNOME

Stock GNOME Shell caps notification banners at roughly 4 seconds and ignores the
app's requested duration (`notif_timeout_ms` in the config, default 10000,
honored by dunst, KDE and most other daemons). On GNOME your options are:

- read missed notifications in the clock menu (they collect there), or
- install the [Notification Timeout](https://extensions.gnome.org/extension/3795/notification-timeout/)
  GNOME extension to lengthen banners system-wide.

A richer alternative (drawing our own now-playing card) is tracked in the issues.

### Windows

A cross-built tray binary exists (`make windows`) but is untested on real
hardware; ship `fipindicateur.exe` next to `libmpv-2.dll`. See the
[CHANGELOG](../CHANGELOG.md) for status.
