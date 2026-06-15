import subprocess

import manager


def test_dashboard_domain_validation_rejects_ip_addresses() -> None:
    assert not manager.is_valid_domain("1.2.3.4")
    assert not manager.is_valid_domain("2001:db8::1")


def test_dashboard_domain_validation_accepts_dns_names() -> None:
    assert manager.is_valid_domain("monitor.example.com")
    assert manager.is_valid_domain("MONITOR.EXAMPLE.COM.")


def test_dashboard_domain_validation_rejects_invalid_names() -> None:
    assert not manager.is_valid_domain("localhost")
    assert not manager.is_valid_domain("bad_name.example.com")
    assert not manager.is_valid_domain("-bad.example.com")


def test_reset_agent_settings_skips_installation_and_resets_traffic_state(tmp_path, monkeypatch) -> None:
    config_path = tmp_path / "agent.toml"
    traffic_state = tmp_path / "traffic-state.json"
    traffic_state.write_text('{"cycle":"old"}', encoding="utf-8")
    systemd_dir = tmp_path / "systemd"
    commands: list[list[str]] = []
    answers = {
        "节点 ID（每台机器必须不同）": "node-1",
        "面板显示名": "Node 1",
        "通信 token": "secret",
        "上报间隔（秒）": "1",
        "每月流量重置日（1-31，留空=不重置）": "15",
        "流量重置小时（0-23）": "4",
        "流量重置分钟（0-59）": "30",
        "月流量上限 GB（留空=无上限）": "500",
    }

    monkeypatch.setattr(manager, "AGENT_CONFIG", config_path)
    monkeypatch.setattr(manager, "AGENT_TRAFFIC_STATE", traffic_state)
    monkeypatch.setattr(manager, "SYSTEMD_DIR", systemd_dir)
    monkeypatch.setattr(manager, "require_root", lambda: True)
    monkeypatch.setattr(manager, "title", lambda _: None)
    monkeypatch.setattr(manager, "pause", lambda *args, **kwargs: None)
    monkeypatch.setattr(manager, "ask_ip", lambda _: "192.0.2.10")
    monkeypatch.setattr(manager, "ask_port", lambda *args: 8080)
    monkeypatch.setattr(manager, "ask", lambda prompt, *args, **kwargs: answers[prompt])

    def fail_install(*args, **kwargs):
        raise AssertionError("resetting Agent settings must not reinstall anything")

    monkeypatch.setattr(manager, "ensure_apt_packages", fail_install)
    monkeypatch.setattr(manager, "ensure_venv", fail_install)
    monkeypatch.setattr(manager, "install_launcher", fail_install)

    def fake_run(args, **kwargs):
        commands.append(args)
        return subprocess.CompletedProcess(args, 0)

    monkeypatch.setattr(manager, "run", fake_run)

    manager.configure_agent(local=False, install=False)

    content = config_path.read_text(encoding="utf-8")
    assert 'server_url = "http://192.0.2.10:8080"' in content
    assert "traffic_reset_day = 15" in content
    assert "traffic_reset_minute = 30" in content
    assert "traffic_limit_gb = 500.0" in content
    assert "traffic_offset_gb" not in content
    assert not traffic_state.exists()
    assert not (systemd_dir / f"{manager.AGENT_SERVICE}.service").exists()
    assert ["systemctl", "stop", manager.AGENT_SERVICE] in commands
    assert ["systemctl", "restart", manager.AGENT_SERVICE] in commands
    assert not any(command[:2] == ["systemctl", "enable"] for command in commands)
