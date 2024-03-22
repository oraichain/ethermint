let
  pkgs = import ../../../nix { };
  fetchEthermint = rev: builtins.fetchTarball "https://github.com/Kava-Labs/ethermint/archive/${rev}.tar.gz";
  released = pkgs.buildGo121Module rec {
    name = "ethermintd";
    src = fetchEthermint "f95892274ca11b6a1dcd66a1d1bc90710e3f45e6";
    subPackages = [ "cmd/ethermintd" ];
    vendorSha256 = "sha256-5JXFuwzTYPc71uPK8OMtc6dmlF6/Lj4TcMCP3GTG76U=";
    doCheck = false;
  };
  current = pkgs.callPackage ../../../. { };
in
pkgs.linkFarm "upgrade-test-package" [
  { name = "genesis"; path = released; }
  { name = "integration-test-upgrade"; path = current; }
]
