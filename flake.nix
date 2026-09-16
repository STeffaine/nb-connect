{
  description = "nbcon - NetBox connection manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.default = pkgs.buildGoModule {
          name = "nbcon";
          version = "0.1.0";
          src = self;
          subPackages = [ "cmd/nbcon" ];
          vendorHash = "sha256-DwCuAxK5Jn3LWTzofuQbLeIhl38UMPkZue6l9tDoSWQ=";
          proxyVendor = true;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [ go gopls gotools ];
        };
      }
    ) // {
      nixosModules.default = { config, pkgs, inputs, ... }:
        {
          environment.systemPackages = [ inputs.nbcon.packages.${pkgs.system}.default ];
        };
    };
}
