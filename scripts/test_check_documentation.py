"""Regression checks for the documentation verifier; no public network access."""

import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

CHECKER = Path(__file__).with_name("check-documentation.py")
spec = importlib.util.spec_from_file_location("documentation_checker", CHECKER)
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)


class DocumentationVerifierTests(unittest.TestCase):
    def test_server_cannot_choose_a_local_file_for_independent_fetch(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "document.json"
            raw = b'{"paths":{},"components":{"schemas":{}}}'
            path.write_bytes(raw)
            source = {"url": path.as_uri(), "sha256": hashlib.sha256(raw).hexdigest()}
            with self.assertRaises((AssertionError, ValueError, RuntimeError)):
                checker.check_all_documentation(lambda *args: None, source)

    @unittest.skipIf(sys.platform == "win32", "fake executable uses a POSIX shebang")
    def test_optimized_python_rejects_a_wrong_tool_catalog(self):
        with tempfile.TemporaryDirectory() as directory:
            binary = Path(directory) / "fake-server"
            binary.write_text("#!" + sys.executable + "\n" + '''
import json, sys
for line in sys.stdin:
    request=json.loads(line)
    if "id" not in request: continue
    method=request["method"]
    if method=="initialize": result={"protocolVersion":"2024-11-05","capabilities":{}}
    elif method=="tools/list": result={"tools":[]}
    elif method=="resources/list": result={"resources":[]}
    else: result={"isError":True,"content":[]}
    print(json.dumps({"jsonrpc":"2.0","id":request["id"],"result":result}),flush=True)
''')
            binary.chmod(0o700)
            result = subprocess.run([sys.executable, "-O", str(CHECKER), str(binary)], capture_output=True, text=True, timeout=10)
            self.assertNotEqual(result.returncode, 0, result.stdout)
            self.assertNotIn('"protocol": "passed"', result.stdout)


if __name__ == "__main__":
    unittest.main()
