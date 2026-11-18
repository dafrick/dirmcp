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
      "/srv/ro/a.md".f = {
        mode = "0644";
        argument = "alpha";
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
        print(machine.execute("journalctl -u dirmcp-ro -u dirmcp-rw --no-pager -o cat | tail -40")[1])
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
        machine.succeed("grep -qx alpha /srv/ro/a.md")
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

    with subtest("files created by the service are group-readable"):
        machine.succeed("stat -c %a /srv/ro/Inbox/new.md | grep -E '^6[0-7]0$'")
  '';
}
