{
  description = "dbview — terminal database viewer (SQLite, MySQL, PostgreSQL, MongoDB, Redis)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        version = "0.1.0";
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "dbview";
          inherit version;
          src = ./.;
          vendorHash = null; # set after first build, see note below
          ldflags = [ "-s" "-w" ];
          meta = with pkgs.lib; {
            description = "Terminal TUI database viewer for SQLite, MySQL, PostgreSQL, MongoDB, and Redis";
            license = licenses.mit;
            mainProgram = "dbview";
          };
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools
          ];
        };

        apps.default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/dbview";
        };
      });
}
