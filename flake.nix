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
      nixosModules.default = { config, lib, pkgs, ... }:
        let
          cfg = config.services.nbcon;
          yamlFormat = pkgs.formats.yaml { };
          renderedConfig = yamlFormat.generate "nbcon-config.yaml" cfg.settings;
          renderedCredentials = if cfg.credentials != null then yamlFormat.generate "nbcon-credentials.yaml" cfg.credentials else null;
          credentialsPath = if cfg.credentialsFile != null then cfg.credentialsFile else "/etc/nbcon/credentials.yaml";
        in
        {
          options.services.nbcon = {
            enable = lib.mkEnableOption "nbcon NetBox connection utility";

            package = lib.mkOption {
              type = lib.types.package;
              default = self.packages.${pkgs.system}.default;
              description = "nbcon package to install";
            };

            settings = lib.mkOption {
              type = yamlFormat.type;
              default = { };
              description = "Contents of nbcon public configuration as YAML data";
            };

            credentials = lib.mkOption {
              type = lib.types.nullOr yamlFormat.type;
              default = null;
              description = "Contents of nbcon credentials configuration as YAML data (stored in Nix store; not suitable for production secrets)";
            };

            credentialsFile = lib.mkOption {
              type = lib.types.nullOr lib.types.str;
              default = null;
              example = "/run/secrets/nbcon-credentials.yaml";
              description = "Path to credentials file used for NBCON_CREDENTIALS (recommended for secret management)";
            };

            cachePath = lib.mkOption {
              type = lib.types.nullOr lib.types.str;
              default = null;
              example = "/var/cache/nbcon/services.json";
              description = "Optional cache path exported as NBCON_CACHE";
            };
          };

          config = lib.mkIf cfg.enable {
            assertions = [
              {
                assertion = cfg.credentials == null || cfg.credentialsFile == null;
                message = "services.nbcon.credentials and services.nbcon.credentialsFile cannot both be set";
              }
              {
                assertion = cfg.credentials != null || cfg.credentialsFile != null;
                message = "set either services.nbcon.credentials or services.nbcon.credentialsFile";
              }
            ];

            environment.systemPackages = [ cfg.package ];

            environment.etc."nbcon/config.yaml".source = renderedConfig;
            environment.etc."nbcon/credentials.yaml".source = lib.mkIf (cfg.credentials != null) renderedCredentials;

            environment.variables = {
              NBCON_CONFIG = "/etc/nbcon/config.yaml";
              NBCON_CREDENTIALS = credentialsPath;
            } // lib.optionalAttrs (cfg.cachePath != null) {
              NBCON_CACHE = cfg.cachePath;
            };
          };
        };
    };
}
