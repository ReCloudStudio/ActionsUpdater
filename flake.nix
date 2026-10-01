{
  description = "GitHub Actions reference updater";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs, ... }:
    let
      systems = [
        "aarch64-darwin"
        "aarch64-linux"
        "x86_64-darwin"
        "x86_64-linux"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (system: {
        default = nixpkgs.legacyPackages.${system}.buildGoModule {
          pname = "actions-updater";
          version = "1.0.1";
          src = ./.;
          subPackages = [ "cmd/actions-updater" ];
          vendorHash = "sha256-s79+Pf1KmxsiuafK07bwHVqz98D5TYfpp1RAGWySbxw=";

          meta = {
            description = "Update GitHub Actions references to their newest tags";
            homepage = "https://github.com/ReCloudStudio/ActionsUpdater";
            license = nixpkgs.lib.licenses.agpl3Plus;
            mainProgram = "actions-updater";
          };
        };
        au = self.packages.${system}.default;
      });

      apps = forAllSystems (system: {
        au = {
          type = "app";
          program = "${nixpkgs.legacyPackages.${system}.lib.getExe self.packages.${system}.au}";
        };
        default = {
          type = "app";
          program = "${nixpkgs.legacyPackages.${system}.lib.getExe self.packages.${system}.default}";
        };
      });

      devShells = forAllSystems (system: {
        default = nixpkgs.legacyPackages.${system}.mkShell {
          packages = with nixpkgs.legacyPackages.${system}; [
            go
            gopls
            gotools
            nixfmt-rfc-style
          ];
        };
      });
    };
}
