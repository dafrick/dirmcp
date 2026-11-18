# VM test: the module runs a read-only instance (with create in Inbox/) and a
# writable one; a client with curl exercises auth, the sandbox and the tools.
{ pkgs, module }:
pkgs.testers.runNixOSTest {
  name = "dirmcp";
  nodes.machine = {
    imports = [ module ];
    environment.systemPackages = [
      pkgs.curl
      pkgs.jq
    ];
    systemd.tmpfiles.settings."10-dirmcp-test" = {
      "/srv/ro".d = {
        mode = "0755";
      };
      "/srv/ro/Inbox".d = {
        mode = "0777";
      };
      # 0666: only the sandbox (not file permissions) can stop writes here.
      "/srv/ro/a.md".f = {
        mode = "0666";
        argument = "alpha";
      };
      "/home/someone".d = {
        mode = "0755";
      };
      "/home/someone/secret.md".f = {
        mode = "0644";
        argument = "home secret";
      };
      "/srv/rw".d = {
        mode = "0777";
      };
      "/srv/rw/b.md".f = {
        mode = "0666";
        argument = "#old";
      };
    };
    # Stand-in for a sops secret: root-only.
    environment.etc."dirmcp/token" = {
      text = "0123456789abcdef0123456789abcdef0123456789abcdef"; # gitleaks:allow (test value)
      mode = "0400";
    };
    services.dirmcp.instances = {
      ro = {
        root = "/srv/ro";
        listen = "127.0.0.1:8091";
        tokenFile = "/etc/dirmcp/token";
        createIn = [ "Inbox" ];
      };
      rw = {
        root = "/srv/rw";
        listen = "127.0.0.1:8092";
        tokenFile = "/etc/dirmcp/token";
        writable = true;
      };
    };
  };

  testScript = ''
    import json

    TOKEN = "0123456789abcdef0123456789abcdef0123456789abcdef"  # gitleaks:allow (test value)

    def call(port, tool, args, token=TOKEN):
        body = json.dumps({"jsonrpc": "2.0", "id": 1, "method": "tools/call",
                           "params": {"name": tool, "arguments": args}})
        out = machine.succeed(
            f"curl -s -H 'Content-Type: application/json' "
            f"-H 'Accept: application/json, text/event-stream' "
            f"-H 'Authorization: Bearer {token}' "
            f"--data @- http://127.0.0.1:{port}/mcp <<'EOF'\n{body}\nEOF"
        )
        data = json.loads(out.split("data: ", 1)[1])
        res = data["result"]
        return res["content"][0]["text"], res.get("isError", False)

    def sh(port, cmd):
        return call(port, "sh", {"command": cmd})[0]

    try:
        machine.wait_for_unit("dirmcp-ro.service")
        machine.wait_for_unit("dirmcp-rw.service")
    except Exception:
        print(machine.execute("journalctl -u dirmcp-ro --no-pager -o cat | grep -v -E 'service:|Start|Failed|Scheduled' | sort -u | head -20")[1])
        raise
    machine.wait_for_open_port(8091)
    machine.wait_for_open_port(8092)

    with subtest("auth"):
        code = machine.succeed("curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:8091/mcp")
        assert code == "401", code

    with subtest("token never in the unit or store"):
        machine.fail("grep -r 0123456789abcdef0123456789abcdef /etc/systemd/system/dirmcp-ro.service")
        machine.succeed("systemctl show dirmcp-ro -p Environment | grep -v 0123456789abcdef")

    with subtest("read-only instance"):
        out = sh(8091, "cat a.md")
        assert "alpha" in out and "[exit status 0]" in out, out
        out = sh(8091, "stat /etc/passwd")
        assert "No such file" in out, out
        out = sh(8091, "echo x > a.md")
        assert "read-only" in out, out
        # Writes by programs: stopped by Landlock and the unit, not by DAC.
        for cmd in ["sed -i s/alpha/omega/ a.md", "touch new.md", "cp a.md copy.md", "mkdir d", "mv a.md b.md"]:
            out = sh(8091, cmd)
            assert "[exit status 0]" not in out, (cmd, out)
        machine.succeed("grep -qx alpha /srv/ro/a.md")
        machine.fail("test -e /srv/ro/new.md -o -e /srv/ro/copy.md -o -e /srv/ro/d -o -e /srv/ro/b.md")
        text, err = call(8091, "create", {"path": "Inbox/new.md", "content": "hello"})
        assert not err, text
        text, err = call(8091, "create", {"path": "Inbox/new.md", "content": "again"})
        assert err, text
        machine.succeed("grep -qx hello /srv/ro/Inbox/new.md")
        text, err = call(8091, "create", {"path": "a.md", "content": "x"})
        assert err, text

    with subtest("writable instance"):
        out = sh(8092, "sed -i s/#old/#new/ b.md && cat b.md")
        assert "#new" in out, out
        out = sh(8092, "rm b.md && ls -A .trash")
        assert "b.md." in out, out
        machine.fail("test -e /srv/rw/b.md")
        out = sh(8092, "echo x > /etc/escape")
        assert "[exit status 0]" not in out, out
        # Program writes outside the root, from a writable instance.
        for cmd in ["cp b.md /srv/ro/x.md", "touch /tmp/escape", "mkdir /nix/store/escape", "cp .trash/* /srv/escape"]:
            out = sh(8092, cmd)
            assert "[exit status 0]" not in out, (cmd, out)
        machine.fail("test -e /srv/ro/x.md -o -e /srv/escape -o -e /nix/store/escape")

    with subtest("unit: no capabilities, no homes, no egress"):
        pid = machine.succeed("systemctl show -p MainPID --value dirmcp-ro").strip()
        status = machine.succeed(f"cat /proc/{pid}/status")
        for field in ["CapEff", "CapPrm", "CapAmb"]:
            line = [l for l in status.splitlines() if l.startswith(field)][0]
            assert line.split()[1] == "0000000000000000", line
        machine.succeed(f"nsenter -t {pid} -m test ! -e /home/someone/secret.md")
        out = machine.succeed("systemctl show -p IPAddressDeny --value dirmcp-ro")
        assert "0.0.0.0/0" in out or "any" in out, out
        out = sh(8091, "cat /home/someone/secret.md")
        assert "home secret" not in out, out

    with subtest("files created by the service are group-readable"):
        machine.succeed("stat -c %a /srv/ro/Inbox/new.md | grep -E '^6[0-7]0$'")

    with subtest("never root"):
        machine.succeed("systemctl show -p User --value dirmcp-ro | grep -qx dirmcp-ro")
  '';
}
