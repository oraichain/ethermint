{ poetry2nix, lib, python311, callPackage }:

let poetry2nix = callPackage ./poetry2nix {};
in poetry2nix.mkPoetryEnv {
  projectDir = ../tests/integration_tests;
  python = python311;
  overrides = poetry2nix.overrides.withDefaults (lib.composeManyExtensions [
    (self: super:
      let
        buildSystems = {
          eth-bloom = [ "setuptools" ];
          pystarport = [ "poetry-core" ];
          durations = [ "setuptools" ];
          multitail2 = [ "setuptools" ];
          pytest-github-actions-annotate-failures = [ "setuptools" ];
          flake8-black = [ "setuptools" ];
          multiaddr = [ "setuptools" ];
          pyunormalize = [ "setuptools" ];
        };
      in
      lib.mapAttrs
        (attr: systems: super.${attr}.overridePythonAttrs
          (old: {
            nativeBuildInputs = (old.nativeBuildInputs or [ ]) ++ map (a: self.${a}) systems;
          }))
        buildSystems
    )
    (self: super: {
      eth-bloom = super.eth-bloom.overridePythonAttrs {
        preConfigure = ''
          substituteInPlace setup.py --replace \'setuptools-markdown\' ""
        '';
      };
#      pyyaml-include = super.pyyaml-include.overridePythonAttrs {
#        preConfigure = ''
#          substituteInPlace setup.py --replace "setup()" "setup(version=\"1.3\")"
#        '';
#      };
    })
  ]);
}
