from __future__ import annotations

import re
from collections import Counter
from dataclasses import dataclass, field
from typing import Any, Callable


@dataclass(frozen=True)
class ImageReference:
    block_id: str
    token: str
    name: str
    mime_type: str
    width: int | None
    height: int | None
    declared_size: int | None
    caption: str


@dataclass(frozen=True)
class ImageResolution:
    markdown_path: str | None
    alt_text: str
    asset: dict[str, Any] | None = None
    warning: str | None = None


@dataclass(frozen=True)
class ConversionOptions:
    expand_sheet: Callable[[str], str | list[str]] | None = None
    asset_mode: str = "placeholder"
    resolve_image: Callable[[ImageReference], ImageResolution] | None = None


@dataclass(frozen=True)
class ConversionResult:
    markdown: str
    counts: Counter[str]
    assets: list[dict[str, Any]] = field(default_factory=list)
    warnings: list[str] = field(default_factory=list)


@dataclass(frozen=True)
class Block:
    id: str
    type: str
    parent_id: str | None
    children: list[str]
    text: str
    raw: dict[str, Any]


@dataclass(frozen=True)
class BlockTree:
    blocks: dict[str, Block]
    order: list[str]
    root_id: str | None


def convert_docx_client_vars(
    client_vars_data: dict[str, Any],
    obj_token: str,
    options: ConversionOptions | None = None,
) -> ConversionResult:
    tree = build_block_tree(client_vars_data, obj_token)
    render_options = options or ConversionOptions()
    ordered_counters: dict[str, int] = {}
    assets: list[dict[str, Any]] = []
    warnings: list[str] = []
    seen: set[str] = set()
    parts: list[str] = []
    if tree.root_id:
        root = tree.blocks.get(tree.root_id)
        if root and root.type == "page" and root.text:
            parts.append(f"# {root.text}")

    for block_id in tree.order:
        rendered = render_block(
            tree,
            block_id,
            0,
            seen,
            assets,
            warnings,
            render_options,
            ordered_counters,
        )
        if rendered.strip():
            parts.append(rendered.rstrip())

    markdown = "\n\n".join(parts).rstrip()
    if markdown:
        markdown += "\n"
    return ConversionResult(
        markdown=markdown,
        counts=count_block_types(tree.blocks),
        assets=assets,
        warnings=warnings,
    )


def build_block_tree(client_vars_data: dict[str, Any], obj_token: str) -> BlockTree:
    raw_map = client_vars_data.get("block_map", {})
    if not isinstance(raw_map, dict):
        raw_map = {}

    blocks: dict[str, Block] = {}
    for block_id, entry in raw_map.items():
        if not isinstance(entry, dict):
            continue
        data = entry.get("data", entry)
        if not isinstance(data, dict):
            data = {}
        block_type = str(data.get("type") or entry.get("type") or "unknown")
        blocks[str(block_id)] = Block(
            id=str(block_id),
            type=block_type,
            parent_id=read_parent(data, entry),
            children=read_children(data, entry),
            text=extract_text(data),
            raw=data,
        )

    root_id = find_root_id(blocks, obj_token)
    if root_id and root_id in blocks:
        root = blocks[root_id]
        order = [child_id for child_id in root.children if child_id in blocks]
    else:
        order = [block_id for block_id in blocks if block_id != root_id]
    return BlockTree(blocks=blocks, order=order, root_id=root_id)


def read_parent(data: dict[str, Any], entry: dict[str, Any]) -> str | None:
    parent = data.get("parent_id") or entry.get("parent_id")
    return str(parent) if parent else None


def read_children(data: dict[str, Any], entry: dict[str, Any]) -> list[str]:
    raw = data.get("children", entry.get("children", []))
    if isinstance(raw, list):
        return [str(item) for item in raw]
    if isinstance(raw, dict):
        out: list[str] = []
        for value in raw.values():
            if isinstance(value, list):
                out.extend(str(item) for item in value)
        return out
    return []


def find_root_id(blocks: dict[str, Block], obj_token: str) -> str | None:
    if obj_token in blocks:
        return obj_token
    for block_id, block in blocks.items():
        if block.type == "page":
            return block_id
    for block_id, block in blocks.items():
        if block.parent_id is None:
            return block_id
    return None


def extract_text(data: dict[str, Any]) -> str:
    return extract_attributed_text(data.get("text"))


def extract_attributed_text(text_obj: Any) -> str:
    if text_obj is None:
        return ""
    if isinstance(text_obj, str):
        return text_obj.strip()
    if isinstance(text_obj, list):
        return "".join(str(item.get("text", item)) if isinstance(item, dict) else str(item) for item in text_obj).strip()
    if isinstance(text_obj, dict):
        initial = text_obj.get("initialAttributedTexts", {})
        if isinstance(initial, dict):
            pieces = initial.get("text", {})
            if isinstance(pieces, dict):
                return render_attributed_text(pieces, initial.get("attribs", {}), text_obj.get("apool", {})).strip()
    return ""


def render_attributed_text(
    pieces: dict[str, Any],
    attribs: Any,
    apool: Any,
) -> str:
    return "".join(
        render_text_piece(str(pieces[key]), piece_url(attribs, apool, key))
        for key in sorted(pieces, key=piece_sort_key)
    )


def render_text_piece(text: str, url: str) -> str:
    if not text or not url:
        return text
    return f"[{escape_markdown_link_text(text)}]({url})"


def piece_url(attribs: Any, apool: Any, piece_key: str) -> str:
    if not isinstance(attribs, dict):
        return ""
    attr_text = str(attribs.get(piece_key, ""))
    attr_ids = re.findall(r"\*(\d+)", attr_text)
    if not attr_ids:
        return ""
    return url_for_attrib_ids(apool, attr_ids)


def url_for_attrib_ids(apool: Any, attr_ids: list[str]) -> str:
    if not isinstance(apool, dict):
        return ""
    num_to_attrib = apool.get("numToAttrib", {})
    if not isinstance(num_to_attrib, dict):
        return ""
    for attr_id in attr_ids:
        url = url_from_attrib(num_to_attrib.get(str(attr_id)))
        if url:
            return url
    return ""


def url_from_attrib(value: Any) -> str:
    if isinstance(value, list):
        if len(value) >= 2 and value[0] == "url":
            return str(value[1])
        for item in value:
            url = url_from_attrib(item)
            if url:
                return url
    if isinstance(value, dict):
        for key in ("url", "href", "link"):
            if value.get(key):
                return str(value[key])
    return ""


def escape_markdown_link_text(text: str) -> str:
    return text.replace("[", "\\[").replace("]", "\\]")


def piece_sort_key(value: str) -> tuple[int, int | str]:
    try:
        return (0, int(value))
    except ValueError:
        return (1, value)


def count_block_types(blocks: dict[str, Block]) -> Counter[str]:
    return Counter(block.type for block in blocks.values())


def render_block(
    tree: BlockTree,
    block_id: str,
    depth: int,
    seen: set[str],
    assets: list[dict[str, Any]],
    warnings: list[str],
    options: ConversionOptions,
    ordered_counters: dict[str, int],
) -> str:
    if block_id in seen:
        return ""
    seen.add(block_id)
    block = tree.blocks.get(block_id)
    if block is None:
        return ""

    if block.type == "page":
        return render_children(
            tree,
            block,
            depth,
            seen,
            assets,
            warnings,
            options,
            ordered_counters,
        )
    if block.type.startswith("heading"):
        level_match = re.search(r"(\d+)$", block.type)
        level = min(int(level_match.group(1)) if level_match else 1, 6)
        return f"{'#' * level} {block.text}".rstrip()
    if block.type == "text":
        return block.text
    if block.type == "bullet":
        line = f"{'  ' * depth}- {block.text}".rstrip()
        children = render_children(
            tree,
            block,
            depth + 1,
            seen,
            assets,
            warnings,
            options,
            ordered_counters,
        )
        return "\n\n".join(part for part in [line, children] if part.strip())
    if block.type == "ordered":
        parent_key = block.parent_id or "__root__"
        ordered_counters[parent_key] = ordered_counters.get(parent_key, 0) + 1
        return f"{'  ' * depth}{ordered_counters[parent_key]}. {block.text}".rstrip()
    if block.type == "code":
        language = normalize_code_language(block.raw.get("language"))
        return f"```{language}\n{block.text}\n```"
    if block.type == "todo":
        marker = "x" if read_bool(block.raw.get("checked")) else " "
        return f"{'  ' * depth}- [{marker}] {block.text}".rstrip()
    if block.type == "divider":
        return "---"
    if block.type == "quote_container":
        inner = render_children(
            tree,
            block,
            depth,
            seen,
            assets,
            warnings,
            options,
            ordered_counters,
        )
        if not inner.strip():
            return ">"
        return "\n".join(f"> {line}" if line else ">" for line in inner.splitlines())
    if block.type == "callout":
        children = render_children(
            tree,
            block,
            depth + 1,
            seen,
            assets,
            warnings,
            options,
            ordered_counters,
        )
        return "\n\n".join(part for part in ["[callout]", children.rstrip()] if part.strip())
    if block.type == "sheet":
        token = str(block.raw.get("token", "") or "")
        marker = "[sheet]" if not token else f"[sheet token={token}]"
        if not token or options.expand_sheet is None:
            return marker
        expanded = options.expand_sheet(token)
        expanded_text = "\n".join(expanded) if isinstance(expanded, list) else str(expanded)
        return "\n".join(part for part in [marker, expanded_text.strip()] if part.strip())
    if block.type == "table":
        return render_table(tree, block, seen, assets, warnings, options, ordered_counters)
    if block.type == "image":
        return render_image(block, assets, warnings, options)
    if block.type in {"table_cell", "whiteboard", "mindnote", "isv"}:
        return f"[{block.type}]"

    child_depth = depth + (1 if block.type else 0)
    children = render_children(
        tree,
        block,
        child_depth,
        seen,
        assets,
        warnings,
        options,
        ordered_counters,
    )
    if block.type not in {"unknown", ""}:
        warning = f"unsupported block type: {block.type}"
        if warning not in warnings:
            warnings.append(warning)
    return "\n\n".join(part for part in [block.text, children] if part.strip())


def render_table(
    tree: BlockTree,
    block: Block,
    seen: set[str],
    assets: list[dict[str, Any]],
    warnings: list[str],
    options: ConversionOptions,
    ordered_counters: dict[str, int],
) -> str:
    rows = [str(row_id) for row_id in block.raw.get("rows_id", []) if row_id]
    columns = [str(column_id) for column_id in block.raw.get("columns_id", []) if column_id]
    cell_set = block.raw.get("cell_set", {})
    if not rows or not columns or not isinstance(cell_set, dict):
        return "[table]"

    rendered_rows: list[list[str]] = []
    for row_id in rows:
        rendered_row: list[str] = []
        for column_id in columns:
            cell_id = table_cell_block_id(cell_set, row_id, column_id)
            rendered_row.append(
                render_table_cell(
                    tree,
                    cell_id,
                    seen,
                    assets,
                    warnings,
                    options,
                    ordered_counters,
                )
            )
        rendered_rows.append(rendered_row)

    if not rendered_rows:
        return "[table]"
    header = rendered_rows[0]
    body = rendered_rows[1:]
    lines = [
        markdown_table_row(header),
        markdown_table_row(["---"] * len(columns)),
    ]
    lines.extend(markdown_table_row(row) for row in body)
    return "\n".join(lines)


def table_cell_block_id(cell_set: dict[str, Any], row_id: str, column_id: str) -> str:
    for key in (f"{row_id}_{column_id}", f"{row_id}{column_id}", f"row{row_id}col{column_id}"):
        cell = cell_set.get(key)
        if isinstance(cell, dict) and cell.get("block_id"):
            return str(cell["block_id"])
    for key, cell in cell_set.items():
        if row_id in str(key) and column_id in str(key):
            if isinstance(cell, dict) and cell.get("block_id"):
                return str(cell["block_id"])
    return ""


def render_table_cell(
    tree: BlockTree,
    cell_id: str,
    seen: set[str],
    assets: list[dict[str, Any]],
    warnings: list[str],
    options: ConversionOptions,
    ordered_counters: dict[str, int],
) -> str:
    cell = tree.blocks.get(cell_id)
    if cell is None:
        return ""
    rendered = render_children(
        tree,
        cell,
        0,
        seen,
        assets,
        warnings,
        options,
        ordered_counters,
    )
    return normalize_table_cell(rendered or cell.text)


def render_image(
    block: Block,
    assets: list[dict[str, Any]],
    warnings: list[str],
    options: ConversionOptions,
) -> str:
    if options.resolve_image is None:
        return "[image]"

    image = block.raw.get("image", {})
    image_data = image if isinstance(image, dict) else {}
    reference = ImageReference(
        block_id=block.id,
        token=str(image_data.get("token", "") or ""),
        name=str(image_data.get("name", "") or ""),
        mime_type=str(image_data.get("mimeType", "") or ""),
        width=read_optional_int(image_data.get("width")),
        height=read_optional_int(image_data.get("height")),
        declared_size=read_optional_int(image_data.get("size")),
        caption=extract_attributed_text(image_data.get("caption")),
    )
    try:
        resolution = options.resolve_image(reference)
    except Exception:
        warnings.append("image resolution failed")
        return "[image]"
    if image_resolution_contains_token(resolution, reference.token):
        warnings.append("image resolution rejected unsafe output")
        return "[image]"
    if resolution.asset is not None:
        assets.append(resolution.asset)
    if resolution.warning:
        warnings.append(resolution.warning)
    if not resolution.markdown_path:
        return "[image]"
    return f"![{resolution.alt_text}]({resolution.markdown_path})"


def image_resolution_contains_token(resolution: ImageResolution, token: str) -> bool:
    if not token:
        return False
    return any(
        value_contains_token(value, token)
        for value in (
            resolution.markdown_path,
            resolution.alt_text,
            resolution.asset,
            resolution.warning,
        )
    )


def value_contains_token(value: Any, token: str) -> bool:
    if isinstance(value, str):
        return token in value
    if isinstance(value, dict):
        return any(
            value_contains_token(key, token) or value_contains_token(item, token)
            for key, item in value.items()
        )
    if isinstance(value, (list, tuple, set)):
        return any(value_contains_token(item, token) for item in value)
    return False


def normalize_table_cell(value: str) -> str:
    text = " ".join(line.strip() for line in value.splitlines() if line.strip())
    return text.replace("|", "\\|")


def normalize_code_language(value: Any) -> str:
    if not isinstance(value, str):
        return ""
    return value.strip().split(maxsplit=1)[0]


def read_bool(value: Any) -> bool:
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        return value.strip().lower() in {"1", "true", "yes", "checked", "done"}
    return bool(value)


def read_optional_int(value: Any) -> int | None:
    if value is None or isinstance(value, bool):
        return None
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


def markdown_table_row(values: list[str]) -> str:
    return "| " + " | ".join(values) + " |"


def render_children(
    tree: BlockTree,
    block: Block,
    depth: int,
    seen: set[str],
    assets: list[dict[str, Any]],
    warnings: list[str],
    options: ConversionOptions,
    ordered_counters: dict[str, int],
) -> str:
    parts = [
        render_block(
            tree,
            child_id,
            depth,
            seen,
            assets,
            warnings,
            options,
            ordered_counters,
        )
        for child_id in block.children
    ]
    return "\n\n".join(part.rstrip() for part in parts if part.strip())
