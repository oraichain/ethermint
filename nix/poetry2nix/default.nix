# hello-poetry2nix/poetry2nix/default.nix
{ pkgs ? import <nixpkgs> { }
, ... }:

let
  src = pkgs.fetchFromGitHub {
    owner = "nix-community";
    repo = "poetry2nix";
    rev = "4eb2ac54029af42a001c9901194e9ce19cbd8a40";
    sha256 = "16fi71fpywiqsya1z99kkb14dansyrmkkrb2clzs3b5qqx673wf4";
  };
in import src { inherit pkgs; }
