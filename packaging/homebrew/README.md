# Homebrew tap

`fipindicateur.rb` is a source-build formula for a **personal tap**. `brew
install` compiles it against the user's own `mpv` (which ships `libmpv`), so the
cgo/libmpv link is a non-issue, just like `make build`.

> Status: **formula prepared; tap not created.** No GitHub repo has been made.
> The steps below are for the owner to run when they want the tap live.
>
> macOS honesty note: the app is CI-built on a GitHub macOS runner against
> `brew install mpv`, but has **never been run on real macOS hardware**. The
> formula's `test` block only checks `fipindicateur version`. Treat the tray app
> itself as unverified on macOS.

## Once, to create the tap

Homebrew maps `brew install <user>/<tap>/<formula>` to the GitHub repo
`<user>/homebrew-<tap>`. For `brew install PLNech/tap/fipindicateur`:

1. Create a repo named **`homebrew-tap`** under your account.
2. Add this formula under `Formula/`:

   ```sh
   git clone https://github.com/PLNech/homebrew-tap.git
   mkdir -p homebrew-tap/Formula
   cp packaging/homebrew/fipindicateur.rb homebrew-tap/Formula/
   cd homebrew-tap
   git add Formula/fipindicateur.rb
   git commit -m "fipindicateur 0.3.0"
   git push
   ```

Users then run:

```sh
brew tap PLNech/tap
brew install fipindicateur
# or in one shot:
brew install PLNech/tap/fipindicateur
```

## Validate before pushing

On a Mac (or Linux with Homebrew):

```sh
brew install --build-from-source ./packaging/homebrew/fipindicateur.rb
brew test fipindicateur
brew audit --strict --new fipindicateur   # style + formula-quality lint
brew style packaging/homebrew/fipindicateur.rb
```

## Per-release update

1. Bump `url` to the new tag and refresh `sha256`:

   ```sh
   curl -sL https://github.com/PLNech/fipindicateur/archive/refs/tags/vX.Y.Z.tar.gz \
     | shasum -a 256
   ```

2. Copy the updated formula into the tap, commit, push. `brew bump-formula-pr`
   can automate this against your own tap.

## Notes

- `depends_on "mpv"` pulls libmpv at runtime; `pkg-config` and `go` are
  build-only.
- `head "...", branch: "main"` supports `brew install --HEAD PLNech/tap/fipindicateur`
  for a bleeding-edge build off `main`.
- homebrew-core is out of reach at current project size (self-submitted software
  faces a raised notability bar); the personal tap has no such gate. Revisit
  core only with real third-party adoption. See issue #6.
