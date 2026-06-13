from pathlib import Path
import tempfile
import unittest
from unittest import mock

import manager


class ManagerTests(unittest.TestCase):
    def test_render_agent_config_escapes_values(self) -> None:
        result = manager.render_agent_config(
            {
                "server_url": "http://127.0.0.1:8000",
                "node_id": 'node"01',
                "token": "secret",
                "name": "Node 01",
                "interval": 2,
                "disk_paths": ["/", "/data"],
            }
        )
        self.assertIn('node_id = "node\\"01"', result)
        self.assertIn('interval = 2', result)
        self.assertIn('disk_paths = ["/", "/data"]', result)

    def test_read_env_ignores_comments_and_invalid_lines(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "monitor.env"
            path.write_text("# comment\nTOKEN=abc\ninvalid\nDB=/tmp/db\n", encoding="utf-8")
            self.assertEqual(manager.read_env(path), {"TOKEN": "abc", "DB": "/tmp/db"})

    def test_health_check_rejects_bad_url(self) -> None:
        ok, detail = manager.health_check("not-a-url")
        self.assertFalse(ok)
        self.assertTrue(detail)

    def test_server_url_adds_scheme_and_port(self) -> None:
        self.assertEqual(manager.server_url("1.2.3.4", 8080), "http://1.2.3.4:8080")
        self.assertEqual(manager.server_url("https://monitor.example.com", 443), "https://monitor.example.com:443")

    def test_nginx_value_reads_exact_directive(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "site.conf"
            path.write_text(
                "server {\n  listen 8080;\n  server_name monitor.example.com;\n}\n",
                encoding="utf-8",
            )
            self.assertEqual(manager.nginx_value(path, "listen"), "8080")
            self.assertEqual(manager.nginx_value(path, "server_name"), "monitor.example.com")

    @mock.patch("manager.run")
    @mock.patch("manager.subprocess.run")
    def test_remove_firewall_rules_only_matches_port(self, subprocess_run, command_run) -> None:
        subprocess_run.return_value = mock.Mock(
            stdout=(
                "-A INPUT -p tcp --dport 22 -j ACCEPT\n"
                "-A INPUT -p tcp -s 1.2.3.4 --dport 8080 -j ACCEPT\n"
                "-A INPUT -p tcp --dport 8080 -j DROP\n"
            )
        )
        removed = manager.remove_firewall_port_rules("8080")
        self.assertEqual(removed, 2)
        self.assertEqual(command_run.call_count, 2)
        for call in command_run.call_args_list:
            self.assertIn("8080", call.args[0])
            self.assertNotIn("22", call.args[0])


if __name__ == "__main__":
    unittest.main()
