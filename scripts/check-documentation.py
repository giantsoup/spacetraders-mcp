#!/usr/bin/env python3
"""Check a built server over stdio without game credentials or gameplay calls."""

import argparse
import json
import os
from pathlib import Path
import selectors
import subprocess
import tempfile
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--live", action="store_true", help="Read the public OpenAPI document")
    args = parser.parse_args()
    binary = args.binary.resolve()
    env = {key: value for key, value in os.environ.items() if not key.startswith("SPACETRADERS_")}
    with tempfile.TemporaryDirectory() as directory, tempfile.TemporaryFile(mode="w+") as errors:
        process = subprocess.Popen(
            [str(binary), "--docs-only"], cwd=directory, env=env,
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=errors,
        )
        selector = selectors.DefaultSelector()
        selector.register(process.stdout, selectors.EVENT_READ)
        pending = bytearray()
        request_id = 0

        def send(method, params=None, notify=False):
            nonlocal request_id
            request = {"jsonrpc": "2.0", "method": method}
            if params is not None:
                request["params"] = params
            if not notify:
                request_id += 1
                request["id"] = request_id
            process.stdin.write((json.dumps(request) + "\n").encode())
            process.stdin.flush()
            if notify:
                return
            deadline = time.monotonic() + 35
            while time.monotonic() < deadline:
                while b"\n" in pending:
                    line, _, rest = pending.partition(b"\n")
                    pending[:] = rest
                    response = json.loads(line)
                    if response.get("id") == request_id:
                        return response
                if selector.select(max(0, deadline - time.monotonic())):
                    data = os.read(process.stdout.fileno(), 65536)
                    if not data:
                        errors.seek(0)
                        raise RuntimeError("Server exited: " + errors.read())
                    pending.extend(data)
            raise TimeoutError(method)

        def call(name, arguments):
            response = send("tools/call", {"name": name, "arguments": arguments})
            assert "error" not in response, response
            result = response["result"]
            assert not result.get("isError"), result
            return json.loads(result["content"][0]["text"])

        try:
            initialized = send("initialize", {
                "protocolVersion": "2024-11-05", "capabilities": {},
                "clientInfo": {"name": "documentation-smoke", "version": "1.0"},
            })
            assert "result" in initialized, initialized
            assert "prompts" not in initialized["result"]["capabilities"], initialized
            send("notifications/initialized", notify=True)
            tools = send("tools/list")["result"]["tools"]
            assert {tool["name"] for tool in tools} == {"search_docs", "get_operation", "get_schema"}, tools
            assert all(tool["annotations"]["readOnlyHint"] for tool in tools), tools
            assert send("resources/list")["result"].get("resources", []) == []
            rejected = send("tools/call", {"name": "navigate_ship", "arguments": {}})
            assert "error" in rejected or rejected.get("result", {}).get("isError"), rejected
            report = {"protocol": "passed", "docs_only": True, "tools": [tool["name"] for tool in tools]}
            if args.live:
                found = call("search_docs", {"query": "navigate", "limit": 5})
                assert found["results"], found
                operation = call("get_operation", {"operation_id": found["results"][0]["operation_id"]})
                assert operation["operation"]["operationId"], operation
                schema = call("get_schema", {"name": "ShipNav"})
                assert schema["schema"], schema
                assert found["source"] == operation["source"] == schema["source"], "Snapshot changed unexpectedly"
                report.update(source=found["source"], operation=operation["operation"]["operationId"], schema="ShipNav")
            print(json.dumps(report, indent=2))
        finally:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
            selector.close()
            process.stdin.close()
            process.stdout.close()


if __name__ == "__main__":
    main()
