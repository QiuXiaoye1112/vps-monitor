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
