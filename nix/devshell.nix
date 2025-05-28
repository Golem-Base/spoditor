{ pkgs, perSystem }:
perSystem.devshell.mkShell {
  packages =
    (with pkgs; [
      gnumake
      go
      golangci-lint
      gotools
      just
      k9s
      kind
      kubebuilder
      kubectl
      kubectx
      kubernetes-controller-tools
      operator-sdk
      revive
      setup-envtest
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

  commands = [
    {
      name = "k";
      category = "ops";
      help = "Shorter alias for kubectl";
      command = ''${pkgs.kubectl}/bin/kubectl "$@"'';
    }
    {
      name = "kns";
      category = "ops";
      help = "Switch kubernetes namespaces";
      command = ''kubens "$@"'';
    }
    {
      name = "kvs";
      category = "Ops";
      help = "kubectl view-secret alias";
      command = ''${pkgs.kubectl-view-secret}/bin/kubectl-view-secret "$@"'';
    }
  ];
}
