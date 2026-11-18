# NixOS module: services.dirmcp.instances.<name>, one hardened service each.
# The flake passes packageFor: pkgs -> the dirmcp package for that system.
{ packageFor }:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  inherit (lib) mkOption types;
  cfg = config.services.dirmcp;
  package = cfg.package;

  instance =
    { name, ... }:
    {
      options = {
        root = mkOption {
          type = types.path;
          description = "Directory to serve.";
        };
        listen = mkOption {
          type = types.str;
          default = "127.0.0.1:8090";
          description = "Address of the MCP endpoint (`/mcp`). Keep it on loopback and put an HTTPS proxy in front.";
        };
        tokenFile = mkOption {
          type = types.path;
          description = "File with the bearer token (≥ 32 bytes), readable by root only, e.g. a sops secret. Passed to the service as a systemd credential.";
        };
        writable = mkOption {
          type = types.bool;
          default = false;
          description = "Let programs change files under the root.";
        };
        createIn = mkOption {
          type = types.listOf types.str;
          default = [ ];
          description = "Folders of the root where the `create` tool may add new files.";
        };
        user = mkOption {
          type = types.str;
          default = "dirmcp-${name}";
          description = "User the service runs as. Created by the module unless it is set to an existing user.";
        };
        group = mkOption {
          type = types.str;
          default = "dirmcp-${name}";
          description = "Primary group of the service.";
        };
        extraGroups = mkOption {
          type = types.listOf types.str;
          default = [ ];
          description = "Supplementary groups, e.g. to share access to the root.";
        };
        memoryMax = mkOption {
          type = types.str;
          default = "2G";
          description = "systemd MemoryMax for the service and all programs it starts.";
        };
        tasksMax = mkOption {
          type = types.int;
          default = 256;
          description = "systemd TasksMax for the service and all programs it starts.";
        };
      };
    };

  loopback =
    i:
    lib.hasPrefix "127." i.listen
    || lib.hasPrefix "[::1]" i.listen
    || lib.hasPrefix "localhost:" i.listen;
  writes = i: i.writable || i.createIn != [ ];

  unit = name: i: {
    description = "dirmcp: ${i.root} over MCP (${name})";
    wantedBy = [ "multi-user.target" ];
    after = [ "network.target" ];
    serviceConfig = {
      ExecStart = lib.escapeShellArgs (
        [
          (lib.getExe package)
          "--root"
          i.root
          "--listen"
          i.listen
          "--token-file"
          "%d/token"
        ]
        ++ lib.optional i.writable "--writable"
        ++ lib.concatMap (d: [
          "--create-in"
          d
        ]) i.createIn
      );
      LoadCredential = "token:${i.tokenFile}";
      User = i.user;
      Group = i.group;
      SupplementaryGroups = i.extraGroups;
      Restart = "on-failure";
      UMask = "0007"; # new files stay group-readable (e.g. for a sync client)

      # Kept open for the sandbox: each started program gets its own user,
      # mount and network namespace, mounts and pivots into a new root, and
      # gains CAP_SYS_ADMIN and CAP_SETPCAP inside that user namespace only
      # (dropped before the program starts). The service user holds no
      # capabilities on the host.
      RestrictNamespaces = "user mnt net";
      SystemCallFilter = [
        "@system-service"
        "@mount"
      ];
      CapabilityBoundingSet = [
        "CAP_SYS_ADMIN"
        "CAP_SETPCAP"
      ];

      # Everything else closed.
      AmbientCapabilities = "";
      NoNewPrivileges = true;
      ProtectSystem = "strict";
      ReadWritePaths = lib.optional (writes i) i.root;
      ProtectHome =
        !(lib.hasPrefix "/home/" (toString i.root) || lib.hasPrefix "/root/" (toString i.root));
      PrivateTmp = true;
      PrivateDevices = true;
      ProtectKernelTunables = true;
      ProtectKernelModules = true;
      ProtectKernelLogs = true;
      ProtectControlGroups = true;
      ProtectClock = true;
      ProtectHostname = true;
      ProtectProc = "invisible";
      RestrictRealtime = true;
      RestrictSUIDSGID = true;
      LockPersonality = true;
      SystemCallArchitectures = "native";
      RestrictAddressFamilies = [
        "AF_UNIX"
        "AF_INET"
        "AF_INET6"
      ];
      IPAddressDeny = lib.mkIf (loopback i) "any";
      IPAddressAllow = lib.mkIf (loopback i) "localhost";
      MemoryMax = i.memoryMax;
      TasksMax = i.tasksMax;
    };
  };
in
{
  options.services.dirmcp.package = mkOption {
    type = types.package;
    default = packageFor pkgs;
    defaultText = lib.literalExpression "dirmcp.packages.\${system}.default";
    description = "The dirmcp package. It pins every tool; do not replace it with a plain build.";
  };

  options.services.dirmcp.instances = mkOption {
    type = types.attrsOf (types.submodule instance);
    default = { };
    description = "dirmcp instances. Each serves one directory as `dirmcp-<name>.service`.";
  };

  config = lib.mkIf (cfg.instances != { }) {
    systemd.services = lib.mapAttrs' (
      name: i: lib.nameValuePair "dirmcp-${name}" (unit name i)
    ) cfg.instances;
    users.users = lib.mapAttrs' (
      name: i:
      lib.nameValuePair i.user {
        isSystemUser = true;
        group = i.group;
      }
    ) (lib.filterAttrs (name: i: i.user == "dirmcp-${name}") cfg.instances);
    users.groups = lib.mapAttrs' (name: i: lib.nameValuePair i.group { }) (
      lib.filterAttrs (name: i: i.group == "dirmcp-${name}") cfg.instances
    );
  };
}
