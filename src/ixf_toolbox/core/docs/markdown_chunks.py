from __future__ import annotations

from dataclasses import dataclass
import re


HEADING = re.compile(r"^(#{1,6})\s+(.+?)\s*$")
FENCE = re.compile(r"^\s*(```+|~~~+)")
TABLE_SEPARATOR = re.compile(r"^\s*\|?\s*:?-{3,}")
IMAGE = re.compile(r"!\[[^\]]*\]\(([^)]+)\)")


@dataclass(frozen=True)
class AtomicBlock:
    kind: str
    start_line: int
    end_line: int
    text: str
    heading_level: int | None = None
    heading_title: str = ""


@dataclass(frozen=True)
class MarkdownChunk:
    index: int
    breadcrumb: str
    start_line: int
    end_line: int
    char_count: int
    image_paths: tuple[str, ...]


@dataclass(frozen=True)
class MarkdownOutline:
    selected_heading_level: int | None
    chunks: tuple[MarkdownChunk, ...]


def build_outline(markdown: str, target_chars: int = 12000) -> MarkdownOutline:
    if target_chars <= 0:
        raise ValueError("target_chars must be positive.")
    blocks = parse_atomic_blocks(markdown)
    if not blocks:
        return MarkdownOutline(None, ())
    selected = select_heading_level(blocks)
    ranges = initial_ranges(blocks, selected)
    split_ranges: list[tuple[int, int]] = []
    for start, end in ranges:
        split_ranges.extend(split_range(blocks, start, end, selected or 0, target_chars))

    chunks: list[MarkdownChunk] = []
    for index, (start, end) in enumerate(split_ranges, start=1):
        start_line = blocks[start].start_line
        end_line = blocks[end - 1].end_line
        text = "".join(block.text for block in blocks[start:end])
        chunks.append(
            MarkdownChunk(
                index=index,
                breadcrumb=breadcrumb_for(blocks, start),
                start_line=start_line,
                end_line=end_line,
                char_count=len(text),
                image_paths=tuple(IMAGE.findall(text)),
            )
        )
    return MarkdownOutline(selected, tuple(chunks))


def render_chunk(markdown: str, outline: MarkdownOutline, index: int) -> str:
    if index < 1 or index > len(outline.chunks):
        raise IndexError(f"chunk index out of range: {index}")
    chunk = outline.chunks[index - 1]
    lines = markdown.splitlines(keepends=True)
    return "".join(lines[chunk.start_line - 1 : chunk.end_line])


def parse_atomic_blocks(markdown: str) -> list[AtomicBlock]:
    lines = markdown.splitlines(keepends=True)
    blocks: list[AtomicBlock] = []
    index = 0
    while index < len(lines):
        line = lines[index]
        if not line.strip():
            index += 1
            continue

        fence = FENCE.match(line)
        if fence:
            marker = fence.group(1)
            end = index + 1
            while end < len(lines):
                if lines[end].lstrip().startswith(marker[:3]):
                    end += 1
                    break
                end += 1
            blocks.append(make_block("code", lines, index, end))
            index = end
            continue

        heading = HEADING.match(line.rstrip("\r\n"))
        if heading:
            blocks.append(
                AtomicBlock(
                    kind="heading",
                    start_line=index + 1,
                    end_line=index + 1,
                    text=line,
                    heading_level=len(heading.group(1)),
                    heading_title=heading.group(2).strip(),
                )
            )
            index += 1
            continue

        if is_table_start(lines, index):
            end = index + 2
            while end < len(lines) and lines[end].strip() and "|" in lines[end]:
                end += 1
            blocks.append(make_block("table", lines, index, end))
            index = end
            continue

        if IMAGE.search(line):
            end = index + 1
            while end < len(lines) and lines[end].strip():
                if HEADING.match(lines[end].rstrip("\r\n")) or FENCE.match(lines[end]):
                    break
                end += 1
            blocks.append(make_block("image", lines, index, end))
            index = end
            continue

        end = index + 1
        while end < len(lines) and lines[end].strip():
            if (
                HEADING.match(lines[end].rstrip("\r\n"))
                or FENCE.match(lines[end])
                or is_table_start(lines, end)
                or IMAGE.search(lines[end])
            ):
                break
            end += 1
        blocks.append(make_block("text", lines, index, end))
        index = end
    return blocks


def make_block(kind: str, lines: list[str], start: int, end: int) -> AtomicBlock:
    return AtomicBlock(kind, start + 1, end, "".join(lines[start:end]))


def is_table_start(lines: list[str], index: int) -> bool:
    return (
        index + 1 < len(lines)
        and "|" in lines[index]
        and TABLE_SEPARATOR.match(lines[index + 1]) is not None
    )


def select_heading_level(blocks: list[AtomicBlock]) -> int | None:
    levels = [block.heading_level for block in blocks if block.heading_level is not None]
    if not levels:
        return None
    if sum(level == 1 for level in levels) > 1:
        return 1
    if 2 in levels:
        return 2
    return min(levels)


def initial_ranges(
    blocks: list[AtomicBlock],
    selected: int | None,
) -> list[tuple[int, int]]:
    if selected is None:
        return [(0, len(blocks))]
    boundaries = [
        index
        for index, block in enumerate(blocks)
        if block.heading_level == selected
    ]
    if not boundaries:
        return [(0, len(blocks))]
    starts = ([0] if boundaries[0] > 0 else []) + boundaries
    return [
        (start, starts[index + 1] if index + 1 < len(starts) else len(blocks))
        for index, start in enumerate(starts)
    ]


def split_range(
    blocks: list[AtomicBlock],
    start: int,
    end: int,
    level: int,
    target_chars: int,
) -> list[tuple[int, int]]:
    if block_chars(blocks, start, end) <= target_chars:
        return [(start, end)]
    next_level = level + 1
    deeper = [
        index
        for index in range(start + 1, end)
        if blocks[index].heading_level == next_level
    ]
    if deeper:
        starts = ([start] if deeper[0] > start else []) + deeper
        out: list[tuple[int, int]] = []
        for index, item_start in enumerate(starts):
            item_end = starts[index + 1] if index + 1 < len(starts) else end
            out.extend(split_range(blocks, item_start, item_end, next_level, target_chars))
        return out
    return pack_atomic_blocks(blocks, start, end, target_chars)


def pack_atomic_blocks(
    blocks: list[AtomicBlock],
    start: int,
    end: int,
    target_chars: int,
) -> list[tuple[int, int]]:
    ranges: list[tuple[int, int]] = []
    current_start = start
    current_chars = 0
    current_has_content = False
    for index in range(start, end):
        block = blocks[index]
        block_size = len(block.text)
        isolate_large_atomic = (
            current_chars > 0
            and not current_has_content
            and block.kind in {"code", "table", "image"}
            and block_size > target_chars
        )
        if isolate_large_atomic or (
            current_chars > 0
            and current_has_content
            and current_chars + block_size > target_chars
        ):
            ranges.append((current_start, index))
            current_start = index
            current_chars = 0
            current_has_content = False
        current_chars += block_size
        if block.kind != "heading":
            current_has_content = True
    ranges.append((current_start, end))
    return ranges


def block_chars(blocks: list[AtomicBlock], start: int, end: int) -> int:
    return sum(len(block.text) for block in blocks[start:end])


def breadcrumb_for(blocks: list[AtomicBlock], start: int) -> str:
    stack: dict[int, str] = {}
    for block in blocks[: start + 1]:
        if block.heading_level is None:
            continue
        stack[block.heading_level] = block.heading_title
        for level in list(stack):
            if level > block.heading_level:
                del stack[level]
    return " > ".join(stack[level] for level in sorted(stack))
