{
  description = "le fipindicateur: tiny system-tray client for the FIP (Radio France) webradios";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        # Stamp the flake revision when available, else the current release.
        version =
          if self ? shortRev then self.shortRev
          else if self ? dirtyShortRev then self.dirtyShortRev
          else "0.3.0";

        fipindicateur = pkgs.buildGoModule {
          pname = "fipindicateur";
          inherit version;
          src = self;

          # Fixed-output hash of the vendored Go deps. Only changes when
          # go.mod/go.sum change; regenerate by setting this to
          # pkgs.lib.fakeHash and reading the value the failed build prints.
          vendorHash = "sha256-4dUCx2EuKoELZOUXzX6lSkmzWfScv83EuAtf5qFBIAc=";

          subPackages = [ "cmd/fipindicateur" ];

          # cgo links libmpv, discovered via pkg-config. mpv-unwrapped carries
          # the client library and mpv.pc (the wrapped `mpv` adds scripts we do
          # not need to build against). This is the whole cgo story on Nix:
          # built from source against nixpkgs' own libmpv, no bundling.
          nativeBuildInputs = [ pkgs.pkg-config ];
          buildInputs = [ pkgs.mpv-unwrapped ];

          ldflags = [
            "-X github.com/PLNech/fipindicateur/internal/version.Version=v${version}"
          ];

          # The tray icon (StatusNotifierItem) and zenity dialogs are resolved
          # at runtime from the user's desktop session, not linked at build time
          # (CI builds with libmpv alone), so they are not buildInputs here.

          meta = with pkgs.lib; {
            description = "Tiny system-tray client for the FIP (Radio France) webradios";
            homepage = "https://github.com/PLNech/fipindicateur";
            license = licenses.gpl3Plus;
            mainProgram = "fipindicateur";
            platforms = platforms.linux ++ platforms.darwin;
          };
        };
      in
      {
        packages.default = fipindicateur;
        packages.fipindicateur = fipindicateur;

        apps.default = flake-utils.lib.mkApp {
          drv = fipindicateur;
          name = "fipindicateur";
        };
        apps.fipindicateur = self.apps.${system}.default;

        # Bonus: `nix develop` gives the exact build toolchain (go + libmpv +
        # pkg-config), matching what `make build` needs locally.
        devShells.default = pkgs.mkShell {
          nativeBuildInputs = [ pkgs.pkg-config ];
          buildInputs = [ pkgs.go pkgs.mpv ];
        };
      });
}
