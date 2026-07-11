# AUR packages

Two publish-ready recipes for the [Arch User Repository](https://aur.archlinux.org):

- **`fipindicateur/`** · versioned, builds the source tarball of the latest
  GitHub tag. Bump per release.
- **`fipindicateur-git/`** · VCS package, always builds `main`. Near-zero
  maintenance.

Both build from source against the user's own `mpv` (which ships `libmpv.so` on
Arch), so the cgo/libmpv build is a non-issue by construction: no
cross-compilation, no bundling.

> Status: **prepared, not published.** AUR account registration is currently
> disabled, so these live in-repo until it reopens. Nothing below has been
> pushed to the AUR.

## Contents of each package dir

- `PKGBUILD` · the build recipe.
- `.SRCINFO` · generated metadata (`makepkg --printsrcinfo > .SRCINFO`), required
  by the AUR. Regenerate whenever the `PKGBUILD` changes.

## Local validation (no AUR account needed)

```sh
cd packaging/aur/fipindicateur
makepkg --printsrcinfo > .SRCINFO   # regenerate metadata
namcap PKGBUILD                     # lint the recipe
makepkg -si                         # build + install locally to smoke-test
```

## Publishing (when AUR registration reopens)

1. Register at <https://aur.archlinux.org> and add your SSH public key
   (Account, then My Account, then SSH Public Key).
2. Clone the (empty) package repo. The name is the `pkgname`:

   ```sh
   git clone ssh://aur@aur.archlinux.org/fipindicateur.git
   git clone ssh://aur@aur.archlinux.org/fipindicateur-git.git
   ```

3. Copy `PKGBUILD` and `.SRCINFO` into the clone, commit, push:

   ```sh
   cp packaging/aur/fipindicateur/{PKGBUILD,.SRCINFO} fipindicateur/
   cd fipindicateur
   git add PKGBUILD .SRCINFO
   git commit -m "Initial import: fipindicateur 0.3.0"
   git push
   ```

## Per-release update (versioned package only)

1. Bump `pkgver` (and reset `pkgrel=1`) in `packaging/aur/fipindicateur/PKGBUILD`.
2. Update `sha256sums` for the new tag tarball:

   ```sh
   curl -sL https://github.com/PLNech/fipindicateur/archive/refs/tags/vX.Y.Z.tar.gz \
     | sha256sum
   ```

3. Regenerate `.SRCINFO`, commit both files, push.

The `-git` package needs no per-release action: its `pkgver()` derives the
version from `git describe` at build time.

## Notes

- `depends=('mpv' 'hicolor-icon-theme')`; `optdepends` cover `zenity` (the
  volume/crossfade slider dialogs) and `libappindicator-gtk3` (the GNOME tray
  via the AppIndicator extension).
- These packages install the binary, the `.desktop` launcher, and the hicolor
  icons. They deliberately omit the user-session watchdog `systemd` unit that
  `make install` sets up: it is a belt-and-braces workaround for an already-fixed
  gnome-shell wedge, and auto-enabling a `--user` unit from a package is not
  clean. Run `make install` from a checkout if you want it.
