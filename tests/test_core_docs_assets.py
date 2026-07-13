from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import requests

from ixf_toolbox.core.docs.assets import ImageAssetWriter
from ixf_toolbox.core.docs.converters.docx_markdown import ImageReference


PNG_BYTES = b"\x89PNG\r\n\x1a\n" + b"\x00" * 64


class FakeResponse:
    def __init__(
        self,
        content: bytes,
        *,
        content_type: str = "image/png",
        status_code: int = 200,
        error_detail: str = "",
        chunks: list[bytes] | None = None,
        stream_error: Exception | None = None,
        close_error: Exception | None = None,
    ) -> None:
        self.content = content
        self.headers = {"Content-Type": content_type}
        self.status_code = status_code
        self.error_detail = error_detail
        self.chunks = chunks
        self.stream_error = stream_error
        self.close_error = close_error
        self.closed = False

    def raise_for_status(self) -> None:
        if self.status_code >= 400:
            raise requests.HTTPError(
                f"status={self.status_code} private={self.error_detail}",
                response=self,  # type: ignore[arg-type]
            )

    def iter_content(self, chunk_size: int) -> Any:
        chunks = self.chunks or [
            self.content[index : index + chunk_size]
            for index in range(0, len(self.content), chunk_size)
        ]
        yield from chunks
        if self.stream_error is not None:
            raise self.stream_error

    def close(self) -> None:
        self.closed = True
        if self.close_error is not None:
            raise self.close_error


class FakeSession:
    def __init__(self, responses: list[FakeResponse]) -> None:
        self.responses = responses
        self.requests: list[dict[str, Any]] = []

    def get(self, url: str, **kwargs: Any) -> FakeResponse:
        self.requests.append({"url": url, **kwargs})
        return self.responses.pop(0)


def image_reference(
    token: str = "boxr-private-token",
    *,
    name: str = "architecture.png",
    mime_type: str = "image/png",
    caption: str = "Architecture diagram",
) -> ImageReference:
    return ImageReference(
        block_id="image-block",
        token=token,
        name=name,
        mime_type=mime_type,
        width=1200,
        height=800,
        declared_size=len(PNG_BYTES),
        caption=caption,
    )


def image_writer(
    tmp_path: Path,
    session: FakeSession,
) -> ImageAssetWriter:
    return ImageAssetWriter(
        session=session,  # type: ignore[arg-type]
        origin="https://tenant.example.test",
        referer="https://tenant.example.test/docx/dox-private-document",
        headers={
            "Origin": "https://tenant.example.test",
            "Referer": "https://tenant.example.test/docx/dox-private-document",
            "X-CSRFToken": "private-csrf",
        },
        document_token="dox-private-document",
        output_root=tmp_path,
        asset_group="docx_1",
    )


def test_image_asset_writer_downloads_png_to_safe_relative_path(tmp_path: Path) -> None:
    token = "boxr-private-token"
    session = FakeSession([FakeResponse(PNG_BYTES)])
    writer = image_writer(tmp_path, session)

    resolution = writer.resolve(image_reference(token))

    assert resolution.markdown_path == "assets/docx_1/image-001.png"
    assert resolution.alt_text == "Architecture diagram"
    assert resolution.warning is None
    assert resolution.asset == {
        "path": "assets/docx_1/image-001.png",
        "mimeType": "image/png",
        "width": 1200,
        "height": 800,
        "sizeBytes": len(PNG_BYTES),
        "status": "downloaded",
        "ordinal": 1,
    }
    asset_path = tmp_path / "assets" / "docx_1" / "image-001.png"
    assert asset_path.read_bytes() == PNG_BYTES
    assert not asset_path.with_suffix(".png.part").exists()

    request = session.requests[0]
    assert request["url"] == (
        "https://tenant.example.test/space/api/box/stream/download/all/"
        f"{token}/?mount_node_token=dox-private-document&mount_point=docx_image"
    )
    assert request["headers"]["X-CSRFToken"] == "private-csrf"
    assert request["stream"] is True
    serialized_asset = json.dumps(resolution.asset, sort_keys=True)
    assert token not in serialized_asset
    assert request["url"] not in serialized_asset
    assert "private-csrf" not in serialized_asset


def test_image_asset_writer_deduplicates_repeated_resource_tokens(tmp_path: Path) -> None:
    session = FakeSession([FakeResponse(PNG_BYTES)])
    writer = image_writer(tmp_path, session)

    first = writer.resolve(image_reference())
    second = writer.resolve(image_reference())

    assert first.markdown_path == second.markdown_path
    assert first.asset is not None
    assert second.asset is None
    assert len(session.requests) == 1
    assert len(list((tmp_path / "assets" / "docx_1").iterdir())) == 1


def test_image_asset_writer_preserves_each_duplicate_caption(tmp_path: Path) -> None:
    session = FakeSession([FakeResponse(PNG_BYTES)])
    writer = image_writer(tmp_path, session)

    first = writer.resolve(image_reference(caption="First caption"))
    second = writer.resolve(image_reference(caption="Second caption"))

    assert first.markdown_path == second.markdown_path
    assert first.alt_text == "First caption"
    assert second.alt_text == "Second caption"
    assert len(session.requests) == 1


def test_image_asset_writer_deduplicates_failed_resource_tokens(tmp_path: Path) -> None:
    response = FakeResponse(
        b"private response body",
        content_type="text/plain",
        status_code=403,
        error_detail="private detail",
    )
    session = FakeSession([response])
    writer = image_writer(tmp_path, session)

    first = writer.resolve(image_reference(caption="First caption"))
    second = writer.resolve(image_reference(caption="Second caption"))

    assert first.warning == "image 1 download failed: http_error"
    assert second.warning == "image 1 download failed: http_error"
    assert second.alt_text == "Second caption"
    assert len(session.requests) == 1
    assert response.closed is True


def test_image_asset_writer_returns_safe_http_warning_and_removes_partial_file(
    tmp_path: Path,
) -> None:
    token = "boxr-private-token"
    session = FakeSession(
        [
            FakeResponse(PNG_BYTES),
            FakeResponse(
                b"private response body",
                content_type="text/plain",
                status_code=403,
                error_detail=f"{token} private-csrf",
            ),
        ]
    )
    writer = image_writer(tmp_path, session)
    writer.resolve(image_reference("boxr-first-token"))

    resolution = writer.resolve(image_reference(token))

    assert resolution.markdown_path is None
    assert resolution.asset is None
    assert resolution.warning == "image 2 download failed: http_error"
    serialized = json.dumps(resolution.__dict__, sort_keys=True)
    assert token not in serialized
    assert "private response body" not in serialized
    assert "private-csrf" not in serialized
    assert not list((tmp_path / "assets" / "docx_1").glob("*.part"))


def test_image_asset_writer_rejects_non_image_response(tmp_path: Path) -> None:
    session = FakeSession([FakeResponse(b"<html>private</html>", content_type="text/html")])
    writer = image_writer(tmp_path, session)

    resolution = writer.resolve(image_reference())

    assert resolution.markdown_path is None
    assert resolution.warning == "image 1 download failed: mime_error"
    assert not list((tmp_path / "assets" / "docx_1").glob("*"))


def test_image_asset_writer_rejects_invalid_image_magic(tmp_path: Path) -> None:
    session = FakeSession([FakeResponse(b"not-a-png", content_type="image/png")])
    writer = image_writer(tmp_path, session)

    resolution = writer.resolve(image_reference())

    assert resolution.markdown_path is None
    assert resolution.warning == "image 1 download failed: content_error"
    assert not list((tmp_path / "assets" / "docx_1").glob("*"))


def test_image_asset_writer_rejects_unsafe_filename_fallback(tmp_path: Path) -> None:
    session = FakeSession([FakeResponse(b"<html>private</html>", content_type="image/custom")])
    writer = image_writer(tmp_path, session)

    resolution = writer.resolve(image_reference(name="payload.html"))

    assert resolution.markdown_path is None
    assert resolution.warning == "image 1 download failed: mime_error"
    assert not list((tmp_path / "assets" / "docx_1").glob("*"))


def test_image_asset_writer_validates_safe_filename_fallback_magic(tmp_path: Path) -> None:
    session = FakeSession([FakeResponse(PNG_BYTES, content_type="image/custom")])
    writer = image_writer(tmp_path, session)

    resolution = writer.resolve(image_reference(name="architecture.png"))

    assert resolution.markdown_path == "assets/docx_1/image-001.png"
    assert resolution.asset is not None
    assert resolution.asset["mimeType"] == "image/custom"


def test_image_asset_writer_cleans_partial_file_after_stream_failure(tmp_path: Path) -> None:
    response = FakeResponse(
        PNG_BYTES,
        chunks=[PNG_BYTES[:16]],
        stream_error=requests.ConnectionError("private stream failure"),
    )
    session = FakeSession([response])
    writer = image_writer(tmp_path, session)

    resolution = writer.resolve(image_reference())

    assert resolution.warning == "image 1 download failed: network_error"
    assert not list((tmp_path / "assets" / "docx_1").glob("*.part"))
    assert response.closed is True


def test_image_asset_writer_suppresses_response_close_errors(tmp_path: Path) -> None:
    response = FakeResponse(
        PNG_BYTES,
        close_error=OSError("private close failure"),
    )
    session = FakeSession([response])
    writer = image_writer(tmp_path, session)

    resolution = writer.resolve(image_reference())

    assert resolution.markdown_path == "assets/docx_1/image-001.png"
    assert response.closed is True


def test_image_asset_writer_removes_stale_generated_files_before_download(
    tmp_path: Path,
) -> None:
    asset_dir = tmp_path / "assets" / "docx_1"
    asset_dir.mkdir(parents=True)
    stale_png = asset_dir / "image-001.png"
    stale_html = asset_dir / "image-002.html"
    stale_large_ordinal = asset_dir / "image-1000.webp.part"
    unrelated = asset_dir / "keep.txt"
    stale_png.write_bytes(b"old private image")
    stale_html.write_text("old private content", encoding="utf-8")
    stale_large_ordinal.write_bytes(b"old partial image")
    unrelated.write_text("keep", encoding="utf-8")

    session = FakeSession([FakeResponse(b"private response body", content_type="text/html")])
    writer = image_writer(tmp_path, session)
    resolution = writer.resolve(image_reference())

    assert resolution.warning == "image 1 download failed: mime_error"
    assert not stale_png.exists()
    assert not stale_html.exists()
    assert not stale_large_ordinal.exists()
    assert unrelated.read_text(encoding="utf-8") == "keep"


def test_image_asset_writer_returns_io_error_when_partial_open_fails(
    monkeypatch: Any,
    tmp_path: Path,
) -> None:
    original_open = Path.open

    def fail_partial_open(path: Path, *args: Any, **kwargs: Any) -> Any:
        if path.name.endswith(".part"):
            raise OSError("private write failure")
        return original_open(path, *args, **kwargs)

    monkeypatch.setattr(Path, "open", fail_partial_open)
    session = FakeSession([FakeResponse(PNG_BYTES)])
    writer = image_writer(tmp_path, session)

    resolution = writer.resolve(image_reference())

    assert resolution.warning == "image 1 download failed: io_error"
    assert session.requests


def test_image_asset_writer_returns_io_error_when_atomic_rename_fails(
    monkeypatch: Any,
    tmp_path: Path,
) -> None:
    original_replace = Path.replace

    def fail_partial_replace(path: Path, target: Path) -> Path:
        if path.name.endswith(".part"):
            raise OSError("private rename failure")
        return original_replace(path, target)

    monkeypatch.setattr(Path, "replace", fail_partial_replace)
    session = FakeSession([FakeResponse(PNG_BYTES)])
    writer = image_writer(tmp_path, session)

    resolution = writer.resolve(image_reference())

    assert resolution.warning == "image 1 download failed: io_error"
    assert not list((tmp_path / "assets" / "docx_1").glob("*.part"))


def test_image_asset_writer_preserves_safe_error_when_partial_cleanup_fails(
    monkeypatch: Any,
    tmp_path: Path,
) -> None:
    original_unlink = Path.unlink

    def fail_partial_unlink(path: Path, *args: Any, **kwargs: Any) -> None:
        if path.name.endswith(".part"):
            raise PermissionError("private cleanup failure")
        original_unlink(path, *args, **kwargs)

    monkeypatch.setattr(Path, "unlink", fail_partial_unlink)
    response = FakeResponse(
        PNG_BYTES,
        chunks=[PNG_BYTES[:16]],
        stream_error=requests.ConnectionError("private stream failure"),
    )
    session = FakeSession([response])
    writer = image_writer(tmp_path, session)

    resolution = writer.resolve(image_reference())

    assert resolution.warning == "image 1 download failed: network_error"
    assert response.closed is True
