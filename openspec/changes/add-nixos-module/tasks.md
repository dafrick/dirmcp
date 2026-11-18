## 1. Test first

- [ ] 1.1 `nix/test.nix`: VM with a read-only instance (`createIn = [ "Inbox" ]`) and a writable one; checks: 401 without token, `cat`, `stat /etc/passwd` missing, write refused (read-only), `create` once only, `sed -i` and `rm` → `.trash/` (writable). Wire as `checks.x86_64-linux.vm`

## 2. Module

- [ ] 2.1 `nix/module.nix`: options, users, unit with credentials and hardening; `nixosModules.default` in the flake

## 3. Docs and checks

- [ ] 3.1 README: module usage and "Deploying securely"
- [ ] 3.2 Refine CHANGELOG
- [ ] 3.3 Run `nix flake check` and `scripts/check-release.sh --base origin/main`; update the PR description

## 4. Review and close

- [ ] 4.1 Independent security review of the diff, posted on the PR; fix findings
- [ ] 4.2 Archive, update the PR description
