from pathlib import Path
import tempfile
import unittest

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


if __name__ == "__main__":
    unittest.main()
