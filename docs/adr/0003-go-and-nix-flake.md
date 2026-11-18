# 0003. Go module, built and checked with a Nix flake

In the context of a single-binary server that runs on NixOS and elsewhere, facing toolchain drift and hand-made release artifacts, we decided for a Go module (the GitHub repo and its tags are the package) with a Nix flake for the dev shell, checks and the package, and neglected release binaries and container images, to achieve reproducible builds that NixOS users pin as a flake input and others install with `go install`, accepting that Nix is needed for the local checks.
