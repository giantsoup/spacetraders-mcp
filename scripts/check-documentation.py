#!/usr/bin/env python3
"""Check a built server over stdio without game credentials or gameplay calls."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import selectors
import subprocess
import tempfile
import time
import urllib.request
from urllib.parse import unquote

SOURCE_URL = "https://api.spacetraders.io/v2/documentation/json"
MAX_SOURCE_BYTES = 8 << 20


def require(condition, message="verification failed"):
    if not condition:
        raise AssertionError(message)


class NoRedirects(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, message, headers, new_url):
        raise ValueError("Documentation verification does not follow redirects")


def check_reference_closure(value, components):
    """Every returned local reference must resolve inside the returned components."""
    seen = set()

    def walk(node):
        if isinstance(node, dict):
            ref = node.get("$ref")
            if ref and ref not in seen:
                seen.add(ref)
                require(ref.startswith("#/components/"), ref)
                parts = [part.replace("~1", "/").replace("~0", "~") for part in unquote(ref[2:]).split("/")]
                resolved = {"components": components}
                for part in parts:
                    resolved = resolved[int(part)] if isinstance(resolved, list) else resolved[part]
                walk(resolved)
            for child in node.values():
                walk(child)
        elif isinstance(node, list):
            for child in node:
                walk(child)

    walk(value)


def check_all_documentation(call, source):
    # Fetch documentation independently of the MCP server for a content comparison.
    require(source["url"] == SOURCE_URL, "The server reported an unexpected documentation source")
    request = urllib.request.Request(SOURCE_URL, headers={"Accept": "application/json"})
    with urllib.request.build_opener(NoRedirects()).open(request, timeout=25) as response:
        raw = response.read(MAX_SOURCE_BYTES + 1)
    require(len(raw) <= MAX_SOURCE_BYTES, "Official documentation exceeds 8 MiB")
    require(hashlib.sha256(raw).hexdigest() == source["sha256"], "Official source changed between reads; rerun the check")
    document = json.loads(raw)
    operation_count = 0
    methods = {"get", "put", "post", "delete", "options", "head", "patch", "trace"}
    for path, item in document["paths"].items():
        for method, operation in item.items():
            if method not in methods:
                continue
            operation_id = operation.get("operationId") or method.upper() + " " + path
            found = call("search_docs", {"query": operation_id, "limit": 20})
            require(operation_id in [result["operation_id"] for result in found["results"]], operation_id)
            result = call("get_operation", {"operation_id": operation_id})
            require(result["operation"] == operation, operation_id)
            require(result["path"] == path and result["method"] == method.upper(), operation_id)
            require(result["path_parameters"] == item.get("parameters"), operation_id)
            require(result["default_security"] == document.get("security"), operation_id)
            require(result["source"] == source and not result["unresolved_refs"], operation_id)
            check_reference_closure([result["operation"], result["path_parameters"]], result["components"])
            for group, definitions in result["components"].items():
                for name, definition in (definitions or {}).items():
                    require(definition == document["components"][group][name], (operation_id, group, name))
            operation_count += 1
    schemas = document.get("components", {}).get("schemas", {})
    for name, definition in schemas.items():
        result = call("get_schema", {"name": name})
        require(result["schema"] == definition, name)
        require(result["source"] == source and not result["unresolved_refs"], name)
        check_reference_closure(result["schema"], result["components"])
        for group, definitions in result["components"].items():
            for component_name, component in (definitions or {}).items():
                require(component == document["components"][group][component_name], (name, group, component_name))
    return {"operations_verified": operation_count, "schemas_verified": len(schemas)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path)
    parser.add_argument("--live", action="store_true", help="Read the public OpenAPI document")
    parser.add_argument("--exhaustive", action="store_true", help="Compare every operation and schema against the public source; implies --live")
    parser.add_argument("--protocol", default="2024-11-05", help="MCP protocol version to request")
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
            require("error" not in response, response)
            result = response["result"]
            require(not result.get("isError"), result)
            return json.loads(result["content"][0]["text"])

        try:
            initialized = send("initialize", {
                "protocolVersion": args.protocol, "capabilities": {},
                "clientInfo": {"name": "documentation-smoke", "version": "1.0"},
            })
            require("result" in initialized, initialized)
            require("prompts" not in initialized["result"]["capabilities"], initialized)
            send("notifications/initialized", notify=True)
            tools = send("tools/list")["result"]["tools"]
            require({tool["name"] for tool in tools} == {"search_docs", "get_operation", "get_schema"}, tools)
            require(all(tool["annotations"]["readOnlyHint"] for tool in tools), tools)
            require(send("resources/list")["result"].get("resources", []) == [], "verification failed")
            rejected = send("tools/call", {"name": "navigate_ship", "arguments": {}})
            require("error" in rejected or rejected.get("result", {}).get("isError"), rejected)
            report = {"protocol": "passed", "docs_only": True, "tools": [tool["name"] for tool in tools]}
            report["negotiated_protocol"] = initialized["result"]["protocolVersion"]
            for name, arguments in [
                ("search_docs", {}), ("search_docs", {"query": ""}),
                ("search_docs", {"query": "ship", "limit": 1.5}),
                ("get_operation", {"operation_id": ""}), ("get_schema", {"name": 3}),
            ]:
                response = send("tools/call", {"name": name, "arguments": arguments})
                require("error" in response or response.get("result", {}).get("isError"), response)
            # Bad requests must leave the connection usable.
            require(len(send("tools/list")["result"]["tools"]) == 3, "verification failed")
            if args.live or args.exhaustive:
                found = call("search_docs", {"query": "navigate", "limit": 5})
                require(found["results"], found)
                operation = call("get_operation", {"operation_id": found["results"][0]["operation_id"]})
                require(operation["operation"]["operationId"], operation)
                schema = call("get_schema", {"name": "ShipNav"})
                require(schema["schema"], schema)
                require(found["source"] == operation["source"] == schema["source"], "Snapshot changed unexpectedly")
                report.update(source=found["source"], operation=operation["operation"]["operationId"], schema="ShipNav")
                for name, arguments in [("get_operation", {"operation_id": "not-a-real-operation"}), ("get_schema", {"name": "NotARealSchema"})]:
                    response = send("tools/call", {"name": name, "arguments": arguments})
                    require(response.get("result", {}).get("isError"), response)
                no_results = call("search_docs", {"query": "no_such_spacetraders_documentation_term"})
                require(no_results["results"] == [] and no_results["total"] == 0, no_results)
                if args.exhaustive:
                    report.update(check_all_documentation(call, found["source"]))
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
