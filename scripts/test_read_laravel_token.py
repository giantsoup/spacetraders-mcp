"""Opt-in adapter tests using disposable data and an installed Laravel autoloader."""

import base64
import json
import os
from pathlib import Path
import shutil
import sqlite3
import subprocess
import tempfile
import unittest

PHP = os.environ.get("SPACETRADERS_TEST_PHP") or shutil.which("php")
AUTOLOAD = os.environ.get("SPACETRADERS_TEST_LARAVEL_AUTOLOAD")
HELPER = Path(__file__).with_name("read-laravel-token.php")


@unittest.skipUnless(PHP and AUTOLOAD, "Set SPACETRADERS_TEST_PHP and SPACETRADERS_TEST_LARAVEL_AUTOLOAD for adapter tests")
class LaravelCredentialTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "vendor").mkdir()
        encoded = base64.b64encode(AUTOLOAD.encode()).decode()
        (self.root / "vendor/autoload.php").write_text("<?php require base64_decode('" + encoded + "');")
        self.key = "base64:" + base64.b64encode(os.urandom(32)).decode()
        self.database = self.root / "private & database.sqlite"
        self.db = sqlite3.connect(self.database)
        self.addCleanup(self.db.close)
        self.db.execute("CREATE TABLE agents (id INTEGER PRIMARY KEY, connection_state TEXT, token_version INTEGER, token TEXT)")
        self.configure()

    def configure(self, **overrides):
        env = {"APP_KEY": self.key, "DB_CONNECTION": "sqlite", "DB_DATABASE": str(self.database)}
        env.update(overrides)
        (self.root / ".env").write_text("\n".join(key + "=" + json.dumps(value) for key, value in env.items()))

    def add_agent(self, state="connected", version=1, key=None):
        env = {**os.environ, "TEST_AUTOLOAD": AUTOLOAD, "TEST_KEY": key or self.key}
        code = '''
        require getenv('TEST_AUTOLOAD');
        $key = base64_decode(substr(getenv('TEST_KEY'), 7));
        echo (new Illuminate\\Encryption\\Encrypter($key, 'AES-256-CBC'))->encrypt('synthetic-agent-token', false);
        '''
        result = subprocess.run([PHP, "-r", code], env=env, capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, "Could not create synthetic encrypted fixture")
        self.db.execute("INSERT INTO agents (connection_state,token_version,token) VALUES (?,?,?)", (state, version, result.stdout))
        self.db.commit()

    def read(self, *args):
        before = self.database.read_bytes()
        result = subprocess.run([PHP, str(HELPER), str(self.root), *args], capture_output=True, text=True, timeout=10)
        self.assertEqual(self.database.read_bytes(), before, "Adapter changed the database")
        self.assertNotIn("synthetic-agent-token", result.stderr)
        return result

    def test_connected_agent_and_reset_exclusion(self):
        self.add_agent("reset")
        self.add_agent()
        result = self.read()
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "synthetic-agent-token")
        self.assertEqual(result.stderr, "")

    def test_ambiguous_account_requires_explicit_id(self):
        self.add_agent()
        self.add_agent()
        self.assertEqual(self.read().stdout, "")
        self.assertNotEqual(self.read().returncode, 0)
        self.assertEqual(self.read("2").returncode, 0)

    def test_reset_or_legacy_credentials_are_not_selected(self):
        self.add_agent("reset")
        self.add_agent(version=0)
        self.assertNotEqual(self.read().returncode, 0)
        self.assertEqual(self.read("1").stdout, "")

    def test_bad_key_or_ciphertext_never_leaks(self):
        self.add_agent()
        self.configure(APP_KEY="invalid-private-key")
        result = self.read()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "")
        self.assertNotIn("invalid-private-key", result.stderr)
        self.configure()
        self.db.execute("UPDATE agents SET token = 'PRIVATE-CIPHERTEXT'")
        self.db.commit()
        result = self.read()
        self.assertEqual(result.stdout, "")
        self.assertNotIn("PRIVATE-CIPHERTEXT", result.stderr)

    def test_previous_encryption_key(self):
        previous = "base64:" + base64.b64encode(os.urandom(32)).decode()
        self.add_agent(key=previous)
        self.configure(APP_PREVIOUS_KEYS=previous)
        self.assertEqual(self.read().returncode, 0)

    def test_cached_config_is_authoritative(self):
        self.add_agent()
        config = {"app": {"key": self.key, "cipher": "AES-256-CBC"},
                  "database": {"default": "sqlite", "connections": {"sqlite": {"database": str(self.database)}}}}
        encoded = base64.b64encode(json.dumps(config).encode()).decode()
        cache = self.root / "bootstrap/cache"
        cache.mkdir(parents=True)
        (cache / "config.php").write_text("<?php return json_decode(base64_decode('" + encoded + "'), true);")
        self.configure(APP_KEY="outdated-key")
        self.assertEqual(self.read().returncode, 0)

    def test_unsupported_database_and_missing_database(self):
        self.configure(DB_CONNECTION="mysql")
        self.assertEqual(self.read().stdout, "")
        missing = self.root / "must-not-be-created.sqlite"
        self.configure(DB_DATABASE=str(missing))
        self.assertNotEqual(self.read().returncode, 0)
        self.assertFalse(missing.exists())


if __name__ == "__main__":
    unittest.main()
