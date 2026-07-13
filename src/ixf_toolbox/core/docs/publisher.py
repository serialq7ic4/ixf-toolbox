from __future__ import annotations

from collections import Counter
from dataclasses import dataclass
from pathlib import Path
import random
import re
import string
import time
from typing import Any
from urllib.parse import urlparse
import uuid

import requests

from ixf_toolbox.core.cookies import DEFAULT_COOKIES, load_cookie_objects
from ixf_toolbox.core.docs.reader import DEFAULT_SPACE_API, USER_AGENT, build_session, csrf_from


Spec = tuple[Any, ...]


@dataclass(frozen=True)
class DocxPublishConfig:
    markdown_path: Path
    base_url: str
    cookies_path: Path = Path(DEFAULT_COOKIES)
    space_api: str = DEFAULT_SPACE_API
    member_id: str = ""
    parent_token: str = ""
    title: str = ""
    title_suffix: str = ""
    required_text: tuple[str, ...] = ()
    apply: bool = False


def base36(value: int) -> str:
    chars = "0123456789abcdefghijklmnopqrstuvwxyz"
    if value == 0:
        return "0"
    out = ""
    while value:
        value, remainder = divmod(value, 36)
        out = chars[remainder] + out
    return out


def attrib_for(text: str) -> str:
    if text == "":
        return "*0+0"
    parts = text.split("\n")
    if len(parts) == 1:
        return "*0+" + base36(len(text))
    prefix_len = sum(len(part) + 1 for part in parts[:-1])
    return "*0|" + base36(len(parts) - 1) + "+" + base36(prefix_len) + "*0+" + base36(
        len(parts[-1])
    )


def clean_inline(text: str) -> str:
    return re.sub(r"`([^`]+)`", r"\1", text).strip()


def split_table_row(row: str) -> list[str]:
    return [clean_inline(cell.strip()) for cell in row.strip().strip("|").split("|")]


def parse_markdown(markdown: str) -> tuple[str, list[Spec]]:
    lines = markdown.splitlines()
    if not lines or not lines[0].startswith("# "):
        raise ValueError("Markdown must start with a level-1 title (`# title`).")

    title = lines[0][2:].strip()
    specs: list[Spec] = []
    index = 1

    def is_table_start(position: int) -> bool:
        return (
            position + 1 < len(lines)
            and lines[position].startswith("|")
            and re.match(r"^\|\s*-+", lines[position + 1]) is not None
        )

    while index < len(lines):
        line = lines[index]
        if not line.strip():
            index += 1
            continue

        if line.startswith("```"):
            buffer: list[str] = []
            index += 1
            while index < len(lines) and not lines[index].startswith("```"):
                buffer.append(lines[index])
                index += 1
            if index < len(lines):
                index += 1
            specs.append(("code", "\n".join(buffer)))
            continue

        if is_table_start(index):
            index += 2
            rows: list[tuple[str, str]] = []
            while index < len(lines) and lines[index].startswith("|"):
                cells = split_table_row(lines[index])
                if len(cells) >= 2:
                    rows.append((cells[0], cells[1]))
                index += 1
            specs.append(
                (
                    "callout",
                    [
                        ("text", "关键时间线"),
                        *[("bullet", f"{when}：{event}") for when, event in rows],
                    ],
                    "clock",
                    "23f1",
                )
            )
            continue

        heading_match = re.match(r"^(#{1,9})\s+(.*)$", line)
        if heading_match:
            level = len(heading_match.group(1))
            specs.append((f"heading{level}", clean_inline(heading_match.group(2))))
            index += 1
            continue

        if line.startswith("- "):
            specs.append(("bullet", clean_inline(line[2:])))
            index += 1
            continue

        if re.match(r"^\d+\.\s+", line):
            specs.append(("ordered", clean_inline(re.sub(r"^\d+\.\s+", "", line))))
            index += 1
            continue

        paragraph = [line.strip()]
        index += 1
        while index < len(lines):
            next_line = lines[index]
            if (
                not next_line.strip()
                or next_line.startswith("```")
                or next_line.startswith("#")
                or next_line.startswith("- ")
                or re.match(r"^\d+\.\s+", next_line)
                or next_line.startswith("|")
            ):
                break
            paragraph.append(next_line.strip())
            index += 1

        text = clean_inline(" ".join(item.rstrip() for item in paragraph).strip())
        if not text:
            continue
        if text.startswith("案例类型："):
            specs.append(("callout", [("text", text)], "memo", "1f4dd"))
        elif text == "完整因果链可以收敛为：":
            specs.append(("callout", [("text", text)], "link", "1f517"))
        elif text.startswith(("换句话说", "本质上", "所以")):
            specs.append(("quote", text))
        else:
            specs.append(("text", text))

    return title, specs


class BlockFactory:
    def __init__(self, author: str) -> None:
        self.author = author
        self._used_ids: set[str] = set()
        self._alphabet = string.ascii_letters + string.digits

    def block_id(self) -> str:
        while True:
            candidate = "doxrz" + "".join(random.choice(self._alphabet) for _ in range(22))
            if candidate not in self._used_ids:
                self._used_ids.add(candidate)
                return candidate

    def text_obj(self, text: str) -> dict[str, Any]:
        return {
            "initialAttributedTexts": {
                "text": {"0": text},
                "attribs": {"0": attrib_for(text)},
            },
            "apool": {
                "numToAttrib": {"0": ["author", self.author]},
                "nextNum": 1,
            },
        }

    def base_block(
        self,
        block_type: str,
        parent_id: str,
        text: str | None = None,
    ) -> dict[str, Any]:
        data: dict[str, Any] = {
            "type": block_type,
            "parent_id": parent_id,
            "children": [],
            "comments": [],
            "revisions": [],
            "locked": False,
            "hidden": False,
            "author": self.author,
            "align": "",
        }
        if block_type.startswith("heading") or block_type in {
            "text",
            "bullet",
            "ordered",
            "code",
        }:
            data["text"] = self.text_obj(text or "")
            data["folded"] = False
        return data

    def code_block(self, parent_id: str, text: str) -> dict[str, Any]:
        data = self.base_block("code", parent_id, text.rstrip("\n"))
        data.update(
            {
                "language": "Plain Text",
                "wrap": False,
                "caption": {
                    "text": {
                        "apool": {"nextNum": 0, "numToAttrib": {}},
                        "initialAttributedTexts": {
                            "attribs": {"0": "|1+1"},
                            "text": {"0": "\n"},
                        },
                    }
                },
            }
        )
        return data

    def quote_blocks(
        self,
        parent_id: str,
        text: str,
    ) -> tuple[list[tuple[str, dict[str, Any]]], str]:
        quote_id = self.block_id()
        child_id = self.block_id()
        quote = {
            "type": "quote_container",
            "parent_id": parent_id,
            "children": [child_id],
            "comments": [],
            "revisions": [],
            "locked": False,
            "hidden": False,
            "author": self.author,
        }
        child = self.base_block("text", quote_id, text)
        return [(quote_id, quote), (child_id, child)], quote_id

    def callout_blocks(
        self,
        parent_id: str,
        text_lines: list[tuple[str, str]],
        emoji_id: str,
        emoji_value: str,
    ) -> tuple[list[tuple[str, dict[str, Any]]], str]:
        callout_id = self.block_id()
        child_ids: list[str] = []
        children: list[tuple[str, dict[str, Any]]] = []
        for block_type, text in text_lines:
            child_id = self.block_id()
            child_ids.append(child_id)
            children.append((child_id, self.base_block(block_type, callout_id, text)))
        callout = {
            "type": "callout",
            "parent_id": parent_id,
            "children": child_ids,
            "comments": [],
            "revisions": [],
            "locked": False,
            "hidden": False,
            "author": self.author,
            "background_color": "",
            "border_color": "",
            "text_color": "",
            "align": "left",
            "emoji_id": emoji_id,
            "emoji_value": emoji_value,
        }
        return [(callout_id, callout), *children], callout_id


def build_blocks(
    specs: list[Spec],
    page_id: str,
    factory: BlockFactory,
) -> tuple[list[str], list[tuple[str, dict[str, Any]]]]:
    top_ids: list[str] = []
    entries: list[tuple[str, dict[str, Any]]] = []
    for spec in specs:
        block_type = spec[0]
        if block_type == "quote":
            quote_entries, top_id = factory.quote_blocks(page_id, spec[1])
            top_ids.append(top_id)
            entries.extend(quote_entries)
        elif block_type == "callout":
            _, lines, emoji_id, emoji_value = spec
            callout_entries, top_id = factory.callout_blocks(
                page_id,
                lines,
                emoji_id,
                emoji_value,
            )
            top_ids.append(top_id)
            entries.extend(callout_entries)
        elif block_type == "code":
            block_id = factory.block_id()
            top_ids.append(block_id)
            entries.append((block_id, factory.code_block(page_id, spec[1])))
        else:
            block_id = factory.block_id()
            top_ids.append(block_id)
            entries.append((block_id, factory.base_block(block_type, page_id, spec[1])))
    return top_ids, entries


def summarize_specs(specs: list[Spec]) -> dict[str, int]:
    return dict(Counter(spec[0] for spec in specs))


def find_doc_token(value: Any) -> str | None:
    if isinstance(value, str) and value.startswith("doxrz"):
        return value
    if isinstance(value, dict):
        for key in ("token", "obj_token", "url_token", "node_token", "id"):
            if key in value:
                found = find_doc_token(value[key])
                if found:
                    return found
        for child in value.values():
            found = find_doc_token(child)
            if found:
                return found
    if isinstance(value, list):
        for child in value:
            found = find_doc_token(child)
            if found:
                return found
    return None


def common_headers(base_url: str, csrf_token: str, referer: str) -> dict[str, str]:
    return {
        "User-Agent": USER_AGENT,
        "Origin": base_url,
        "Referer": referer,
        "X-CSRFToken": csrf_token,
    }


def client_vars(
    session: requests.Session,
    space_api: str,
    base_url: str,
    csrf_token: str,
    page_id: str,
) -> dict[str, Any]:
    response = session.get(
        f"{space_api}/space/api/docx/pages/client_vars?id={page_id}&open_type=1",
        headers=common_headers(base_url, csrf_token, f"{base_url}/docx/{page_id}"),
        timeout=30,
    )
    response.raise_for_status()
    payload = response.json()
    if payload.get("code") != 0:
        raise RuntimeError("Could not load the created document state.")
    return payload["data"]


def resolve_member_id(override: str, root: dict[str, Any]) -> str:
    if override:
        return override
    author = str((root.get("data") or {}).get("author") or "")
    if not author:
        raise RuntimeError("Could not determine the authenticated document member identifier.")
    return author


def verify_doc(
    session: requests.Session,
    space_api: str,
    base_url: str,
    csrf_token: str,
    page_id: str,
    required_text: tuple[str, ...],
) -> dict[str, Any]:
    last: dict[str, Any] | None = None
    for _ in range(8):
        time.sleep(0.8)
        vars_after = client_vars(session, space_api, base_url, csrf_token, page_id)
        blocks = vars_after["block_map"]
        counts = Counter(block["data"].get("type") for block in blocks.values())
        text_values = [
            block["data"]
            .get("text", {})
            .get("initialAttributedTexts", {})
            .get("text", {})
            .get("0", "")
            for block in blocks.values()
            if "text" in block["data"]
        ]
        all_text = "\n".join(text_values)
        code_texts = [
            block["data"]
            .get("text", {})
            .get("initialAttributedTexts", {})
            .get("text", {})
            .get("0", "")
            for block in blocks.values()
            if block["data"].get("type") == "code"
        ]
        checks = [
            counts.get("code", 0) > 0 if code_texts else True,
            any("\n" in item for item in code_texts) if code_texts else True,
            all(item in all_text for item in required_text),
        ]
        last = {
            "ok": all(checks),
            "counts": dict(counts),
            "textChars": len(all_text),
        }
        if last["ok"]:
            return last
    return last or {"ok": False, "counts": {}, "textChars": 0}


def validate_base_url(base_url: str) -> str:
    normalized = base_url.rstrip("/")
    parsed = urlparse(normalized)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise ValueError("--base-url must be an absolute HTTP(S) URL.")
    return normalized


def publish_markdown(config: DocxPublishConfig) -> dict[str, object]:
    markdown_path = config.markdown_path.expanduser()
    markdown = markdown_path.read_text(encoding="utf-8")
    source_title, specs = parse_markdown(markdown)
    title = config.title or (source_title + config.title_suffix)
    counts = summarize_specs(specs)
    base_url = validate_base_url(config.base_url)

    if not config.apply:
        return {
            "ok": True,
            "dryRun": True,
            "title": title,
            "counts": counts,
        }

    space_api = config.space_api.rstrip("/")
    cookies = load_cookie_objects(config.cookies_path)
    session = build_session(cookies)
    csrf_token = csrf_from(cookies)
    create_response = session.post(
        f"{space_api}/space/api/explorer/v2/create/object/",
        data={
            "type": "22",
            "source": "0",
            "uuid": str(uuid.uuid4()),
            "name": title,
            "parent_token": config.parent_token,
        },
        headers=common_headers(base_url, csrf_token, f"{base_url}/drive/home/"),
        timeout=30,
    )
    create_response.raise_for_status()
    create_payload = create_response.json()
    if create_payload.get("code") != 0:
        raise RuntimeError("Document creation failed.")

    page_id = find_doc_token(create_payload)
    if not page_id:
        raise RuntimeError("Document creation did not return a document token.")

    final_url = f"{base_url}/docx/{page_id}"
    vars_before = client_vars(session, space_api, base_url, csrf_token, page_id)
    root = vars_before["block_map"][page_id]
    author = str(root["data"]["author"])
    member_id = resolve_member_id(config.member_id, root)
    root_children = list(root["data"].get("children") or [])

    factory = BlockFactory(author)
    top_ids, entries = build_blocks(specs, page_id, factory)
    change_map: dict[str, Any] = {
        page_id: {
            "id": page_id,
            "version": root["version"],
            "payload": {
                "ops": [
                    {
                        "p": ["children", len(root_children) + index],
                        "action": {"li": block_id},
                    }
                    for index, block_id in enumerate(top_ids)
                ]
            },
        }
    }
    for block_id, data in entries:
        change_map[block_id] = {
            "id": block_id,
            "version": 0,
            "payload": {"ops": [{"p": [], "action": {"oi": data}}]},
        }

    write_response = session.post(
        f"{base_url}/space/api/docx/blocks/user_change",
        json={
            "member_id": member_id,
            "uuid": str(uuid.uuid4()),
            "page_id": page_id,
            "change_map": change_map,
        },
        headers={
            **common_headers(base_url, csrf_token, final_url),
            "Content-Type": "application/json",
        },
        timeout=60,
    )
    write_response.raise_for_status()
    write_payload = write_response.json()
    if write_payload.get("code") != 0:
        raise RuntimeError("Document content write failed.")

    verify = verify_doc(
        session,
        space_api,
        base_url,
        csrf_token,
        page_id,
        config.required_text,
    )
    return {
        "ok": bool(verify["ok"]),
        "dryRun": False,
        "title": title,
        "counts": counts,
        "verify": verify,
        "url": final_url,
    }
