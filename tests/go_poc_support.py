from __future__ import annotations

import base64
from collections.abc import Iterator
from contextlib import contextmanager
import gzip
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import subprocess
import threading


ROOT = Path(__file__).resolve().parents[1]
GO_ENV = {
    **os.environ,
    "GOFLAGS": "",
}


def build_go_ixf(tmp_path: Path) -> Path:
    binary = tmp_path / ("ixf-go.exe" if os.name == "nt" else "ixf-go")
    subprocess.run(
        ["go", "build", "-o", str(binary), "./cmd/ixf"],
        cwd=ROOT,
        env=GO_ENV,
        text=True,
        capture_output=True,
        check=True,
    )
    return binary


def run_go_ixf(
    binary: Path,
    *args: str,
    home: Path | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[str]:
    env = dict(GO_ENV)
    if home is not None:
        env["HOME"] = str(home)
    return subprocess.run(
        [str(binary), *args],
        cwd=ROOT,
        env=env,
        text=True,
        capture_output=True,
        check=check,
    )


def write_cookie_fixture(
    path: Path,
    *,
    csrf_token: str = "csrf-fixture",
    session: str = "session-fixture",
) -> None:
    path.write_text(
        json.dumps(
            [
                {"name": "_csrf_token", "value": csrf_token},
                {"name": "session", "value": session},
            ]
        ),
        encoding="utf-8",
    )


@contextmanager
def serve_handler(handler: type[BaseHTTPRequestHandler]) -> Iterator[str]:
    server = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}"
    finally:
        server.shutdown()
        thread.join(timeout=5)


def write_json_response(handler: BaseHTTPRequestHandler, payload: dict) -> None:
    body = json.dumps(payload).encode("utf-8")
    handler.send_response(200)
    handler.send_header("Content-Type", "application/json")
    handler.send_header("Content-Length", str(len(body)))
    handler.end_headers()
    handler.wfile.write(body)


def gzip_json(value: dict) -> str:
    return base64.b64encode(gzip.compress(json.dumps(value).encode("utf-8"))).decode("ascii")


def attributed_text(text: str) -> dict:
    return {"initialAttributedTexts": {"text": {"0": text}}}


def linked_text(label: str, url: str, suffix: str = "") -> dict:
    return {
        "apool": {"numToAttrib": {"0": [["url", url]]}},
        "initialAttributedTexts": {
            "attribs": {"0": "*0+4"},
            "text": {"0": label, "1": suffix},
        },
    }


def block(data: dict) -> dict:
    return {"data": data}


def remote_docx_parity_client_vars(image_token: str) -> dict:
    return {
        "block_map": {
            "page_1": block(
                {
                    "type": "page",
                    "children": [
                        "heading_1",
                        "text_1",
                        "todo_1",
                        "quote_1",
                        "callout_1",
                        "table_1",
                        "image_1",
                        "sheet_1",
                    ],
                    "text": attributed_text("Parity Doc"),
                }
            ),
            "heading_1": block(
                {"type": "heading2", "parent_id": "page_1", "text": attributed_text("Plan")}
            ),
            "text_1": block(
                {
                    "type": "text",
                    "parent_id": "page_1",
                    "text": linked_text("Spec", "https://example.test/spec", " is ready"),
                }
            ),
            "todo_1": block(
                {
                    "type": "todo",
                    "parent_id": "page_1",
                    "checked": False,
                    "text": attributed_text("Ship parity"),
                }
            ),
            "quote_1": block(
                {"type": "quote_container", "parent_id": "page_1", "children": ["quote_text_1"]}
            ),
            "quote_text_1": block(
                {"type": "text", "parent_id": "quote_1", "text": attributed_text("Quote line")}
            ),
            "callout_1": block(
                {"type": "callout", "parent_id": "page_1", "children": ["callout_text_1"]}
            ),
            "callout_text_1": block(
                {"type": "text", "parent_id": "callout_1", "text": attributed_text("Callout body")}
            ),
            "table_1": block(
                {
                    "type": "table",
                    "parent_id": "page_1",
                    "rows_id": ["row_1", "row_2"],
                    "columns_id": ["col_1", "col_2"],
                    "cell_set": {
                        "row_1_col_1": {"block_id": "cell_1_1"},
                        "row_1_col_2": {"block_id": "cell_1_2"},
                        "row_2_col_1": {"block_id": "cell_2_1"},
                        "row_2_col_2": {"block_id": "cell_2_2"},
                    },
                }
            ),
            "cell_1_1": block({"type": "table_cell", "children": ["cell_text_1_1"]}),
            "cell_1_2": block({"type": "table_cell", "children": ["cell_text_1_2"]}),
            "cell_2_1": block({"type": "table_cell", "children": ["cell_text_2_1"]}),
            "cell_2_2": block({"type": "table_cell", "children": ["cell_text_2_2"]}),
            "cell_text_1_1": block(
                {"type": "text", "parent_id": "cell_1_1", "text": attributed_text("Metric")}
            ),
            "cell_text_1_2": block(
                {"type": "text", "parent_id": "cell_1_2", "text": attributed_text("Value")}
            ),
            "cell_text_2_1": block(
                {"type": "text", "parent_id": "cell_2_1", "text": attributed_text("Coverage")}
            ),
            "cell_text_2_2": block(
                {"type": "text", "parent_id": "cell_2_2", "text": attributed_text("100%")}
            ),
            "image_1": block(
                {
                    "type": "image",
                    "parent_id": "page_1",
                    "image": {
                        "token": image_token,
                        "name": "diagram.svg",
                        "mimeType": "image/svg+xml",
                        "width": 640,
                        "height": 360,
                        "size": 22,
                        "caption": attributed_text("Architecture diagram"),
                    },
                }
            ),
            "sheet_1": block(
                {"type": "sheet", "parent_id": "page_1", "token": "shtr_fixture_sheet1"}
            ),
        }
    }


def sheet_expansion_lines() -> list[str]:
    return [
        "[sheet-meta workbook_token=shtr_fixture sheet_id=sheet1 rows=2 cols=2]",
        "```tsv",
        "Name\tValue",
        "Alpha\t42",
        "```",
        "",
    ]


def sheet_client_vars_payload() -> dict:
    return {
        "code": 0,
        "data": {
            "formerlySchema": {
                "clientvars": {
                    "gzip_snapshot": gzip_json(
                        {"sheets": {"sheet1": {"rowCount": 2, "columnCount": 2}}}
                    ),
                    "extra_data": {
                        "blocks": [
                            {
                                "row": 0,
                                "gzip_datatable": gzip_json(
                                    {
                                        "rows": [
                                            {"columns": [{"value": "Name"}, {"value": "Value"}]},
                                            {"columns": [{"value": "Alpha"}, {"value": 42}]},
                                        ]
                                    }
                                ),
                            }
                        ]
                    },
                }
            }
        },
    }
