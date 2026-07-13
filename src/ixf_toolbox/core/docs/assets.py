from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
import re
from typing import Any, Mapping
from urllib.parse import quote, urlencode

import requests

from ixf_toolbox.core.docs.converters.docx_markdown import ImageReference, ImageResolution


MIME_EXTENSIONS = {
    "image/png": ".png",
    "image/jpeg": ".jpg",
    "image/gif": ".gif",
    "image/webp": ".webp",
    "image/svg+xml": ".svg",
    "image/bmp": ".bmp",
    "image/tiff": ".tiff",
}
SUFFIX_MIME_TYPES = {
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".gif": "image/gif",
    ".webp": "image/webp",
    ".svg": "image/svg+xml",
    ".bmp": "image/bmp",
    ".tif": "image/tiff",
    ".tiff": "image/tiff",
}
SAFE_ASSET_GROUP = re.compile(r"^[a-zA-Z0-9_-]+$")
GENERATED_IMAGE_FILE = re.compile(r"^image-\d{3,}\.[a-z0-9]{1,8}(?:\.part)?$")


@dataclass(frozen=True)
class CachedImage:
    markdown_path: str | None
    failure_reason: str | None
    ordinal: int


class ImageAssetWriter:
    def __init__(
        self,
        *,
        session: requests.Session,
        origin: str,
        referer: str,
        headers: Mapping[str, str],
        document_token: str,
        output_root: Path,
        asset_group: str,
    ) -> None:
        if not SAFE_ASSET_GROUP.fullmatch(asset_group):
            raise ValueError("asset_group must contain only letters, numbers, underscores, or dashes.")
        self.session = session
        self.origin = origin.rstrip("/")
        self.referer = referer
        self.headers = dict(headers)
        self.document_token = document_token
        self.output_root = output_root
        self.asset_group = asset_group
        self.asset_dir = output_root / "assets" / asset_group
        self._resolved: dict[str, CachedImage] = {}
        self._next_ordinal = 1
        cleanup_stale_generated_files(self.asset_dir)

    def resolve(self, reference: ImageReference) -> ImageResolution:
        cached = self._resolved.get(reference.token)
        if cached is not None:
            alt_text = image_alt_text(reference, cached.ordinal)
            if cached.markdown_path is not None:
                return ImageResolution(markdown_path=cached.markdown_path, alt_text=alt_text)
            assert cached.failure_reason is not None
            return failed_resolution(cached.ordinal, alt_text, cached.failure_reason)

        ordinal = self._next_ordinal
        self._next_ordinal += 1
        alt_text = image_alt_text(reference, ordinal)
        if not reference.token:
            return failed_resolution(ordinal, alt_text, "missing_token")

        query = urlencode(
            {
                "mount_node_token": self.document_token,
                "mount_point": "docx_image",
            }
        )
        url = (
            f"{self.origin}/space/api/box/stream/download/all/"
            f"{quote(reference.token, safe='')}/?{query}"
        )
        try:
            response = self.session.get(
                url,
                headers=self.headers,
                timeout=60,
                stream=True,
            )
        except requests.RequestException:
            return self._failure(reference, ordinal, alt_text, "network_error")

        try:
            try:
                response.raise_for_status()
            except requests.RequestException:
                return self._failure(reference, ordinal, alt_text, "http_error")

            mime_type = normalize_mime_type(response.headers.get("Content-Type"))
            image_format = resolve_image_format(mime_type, reference.name)
            if image_format is None:
                return self._failure(reference, ordinal, alt_text, "mime_error")
            extension, validation_mime_type = image_format

            filename = f"image-{ordinal:03d}{extension}"
            final_path = self.asset_dir / filename
            partial_path = final_path.with_name(f"{filename}.part")
            prefix = bytearray()
            size_bytes = 0
            try:
                self.asset_dir.mkdir(parents=True, exist_ok=True)
                with partial_path.open("wb") as handle:
                    for chunk in response.iter_content(chunk_size=64 * 1024):
                        if not chunk:
                            continue
                        handle.write(chunk)
                        size_bytes += len(chunk)
                        if len(prefix) < 512:
                            prefix.extend(chunk[: 512 - len(prefix)])
            except requests.RequestException:
                remove_if_exists(partial_path)
                return self._failure(reference, ordinal, alt_text, "network_error")
            except OSError:
                remove_if_exists(partial_path)
                return self._failure(reference, ordinal, alt_text, "io_error")

            if not has_valid_image_magic(validation_mime_type, bytes(prefix)):
                remove_if_exists(partial_path)
                return self._failure(reference, ordinal, alt_text, "content_error")

            try:
                partial_path.replace(final_path)
            except OSError:
                remove_if_exists(partial_path)
                return self._failure(reference, ordinal, alt_text, "io_error")

            markdown_path = (Path("assets") / self.asset_group / filename).as_posix()
            asset = {
                "path": markdown_path,
                "mimeType": mime_type,
                "width": reference.width,
                "height": reference.height,
                "sizeBytes": size_bytes,
                "status": "downloaded",
                "ordinal": ordinal,
            }
            self._resolved[reference.token] = CachedImage(markdown_path, None, ordinal)
            return ImageResolution(
                markdown_path=markdown_path,
                alt_text=alt_text,
                asset=asset,
            )
        finally:
            close_safely(response)

    def _failure(
        self,
        reference: ImageReference,
        ordinal: int,
        alt_text: str,
        reason: str,
    ) -> ImageResolution:
        if reference.token:
            self._resolved[reference.token] = CachedImage(None, reason, ordinal)
        return failed_resolution(ordinal, alt_text, reason)


def normalize_mime_type(value: Any) -> str:
    return str(value or "").split(";", 1)[0].strip().lower()


def resolve_image_format(mime_type: str, original_name: str) -> tuple[str, str] | None:
    extension = MIME_EXTENSIONS.get(mime_type)
    if extension:
        return extension, mime_type
    suffix = Path(original_name).suffix.lower()
    validation_mime_type = SUFFIX_MIME_TYPES.get(suffix)
    if mime_type.startswith("image/") and validation_mime_type:
        return suffix, validation_mime_type
    return None


def image_alt_text(reference: ImageReference, ordinal: int) -> str:
    if reference.caption.strip():
        return reference.caption.strip()
    stem = Path(reference.name).stem.strip()
    return stem or f"image {ordinal}"


def failed_resolution(ordinal: int, alt_text: str, reason: str) -> ImageResolution:
    return ImageResolution(
        markdown_path=None,
        alt_text=alt_text,
        warning=f"image {ordinal} download failed: {reason}",
    )


def remove_if_exists(path: Path) -> None:
    try:
        path.unlink()
    except OSError:
        return


def close_safely(response: Any) -> None:
    try:
        response.close()
    except Exception:
        return


def cleanup_stale_generated_files(asset_dir: Path) -> None:
    try:
        children = list(asset_dir.iterdir())
    except OSError:
        return
    for child in children:
        if child.is_file() and GENERATED_IMAGE_FILE.fullmatch(child.name):
            remove_if_exists(child)


def has_valid_image_magic(mime_type: str, prefix: bytes) -> bool:
    if mime_type == "image/png":
        return prefix.startswith(b"\x89PNG\r\n\x1a\n")
    if mime_type == "image/jpeg":
        return prefix.startswith(b"\xff\xd8\xff")
    if mime_type == "image/gif":
        return prefix.startswith((b"GIF87a", b"GIF89a"))
    if mime_type == "image/webp":
        return len(prefix) >= 12 and prefix.startswith(b"RIFF") and prefix[8:12] == b"WEBP"
    if mime_type == "image/svg+xml":
        text = prefix.lstrip().lower()
        return text.startswith(b"<svg") or (text.startswith(b"<?xml") and b"<svg" in text)
    if mime_type == "image/bmp":
        return prefix.startswith(b"BM")
    if mime_type == "image/tiff":
        return prefix.startswith((b"II*\x00", b"MM\x00*"))
    return False
