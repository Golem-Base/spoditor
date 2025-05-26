{ pkgs, perSystem }:
perSystem.devshell.mkShell {
  packages =
    (with pkgs; [
      just
      go
      templ
    ])
    ++ (with perSystem.self; [ formatter ]);

  env = [
    {
      name = "NIX_PATH";
      value = "nixpkgs=${toString pkgs.path}";
    }
    {
      name = "NIX_DIR";
      eval = "$PRJ_ROOT/nix";
    }
  ];

  commands = [ ];
}
