# hello-poetry2nix/poetry2nix/default.nix
{ pkgs ? import <nixpkgs> { }
, ... }:

let
  src = pkgs.fetchFromGitHub {
    owner = "nix-community";
    repo = "poetry2nix";
    rev = "e0b44e9e2d3aa855d1dd77b06f067cd0e0c3860d";
    sha256 = "sha256-puYyylgrBS4AFAHeyVRTjTUVD8DZdecJfymWJe7H438=";
  };
in import src { inherit pkgs; }
