from types import SimpleNamespace
import unittest

import server


class ServerTests(unittest.TestCase):
    def test_request_client_ip_prefers_forwarded_ip(self) -> None:
        request = SimpleNamespace(
            headers={"x-forwarded-for": "203.0.113.8, 127.0.0.1", "x-real-ip": "203.0.113.9"},
            client=SimpleNamespace(host="127.0.0.1"),
        )
        self.assertEqual(server.request_client_ip(request), "203.0.113.8")

    def test_request_client_ip_skips_invalid_headers(self) -> None:
        request = SimpleNamespace(
            headers={"x-forwarded-for": "invalid"},
            client=SimpleNamespace(host="198.51.100.4"),
        )
        self.assertEqual(server.request_client_ip(request), "198.51.100.4")


if __name__ == "__main__":
    unittest.main()
