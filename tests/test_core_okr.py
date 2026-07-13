from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import pytest

from ixf_toolbox.core.okr import (
    detect_okr_url,
    read_okr,
    read_okr_url,
)


OKR_URL = (
    "https://example.xfchat.iflytek.com/okr/user/owner-fixture/"
    "?lang=zh-CN&okrId=okr-fixture-200&type=leader"
)


@dataclass
class FakeResponse:
    payload: dict[str, Any]

    def raise_for_status(self) -> None:
        return None

    def json(self) -> dict[str, Any]:
        return self.payload


class FakeCookies:
    def __init__(self) -> None:
        self.values: dict[str, str] = {}

    def get(self, name: str, default: str = "") -> str:
        return self.values.get(name, default)


class FakeSession:
    def __init__(self, payload: dict[str, Any] | None = None) -> None:
        self.cookies = FakeCookies()
        self.payload = payload or {
            "code": 0,
            "message": "",
            "okr_detail_data": {
                "name": "2026 年 7 月 - 9 月",
                "owner_info": {
                    "user_info": {
                        "locale_names": {"zh": "Fixture Owner"},
                    }
                },
                "objective_list": [
                    {
                        "id": "o1",
                        "name": {"blocks": [{"text": "支撑计算平台规模化落地"}]},
                        "kr_list": [
                            {
                                "id": "kr1",
                                "content": {"blocks": [{"text": "SAE 生产应用数提升到 8000"}]},
                                "progress_rate": {"percent": 20},
                            },
                            {
                                "id": "kr2",
                                "content_v2": {
                                    "0": {
                                        "ops": [
                                            {
                                                "insert": "KubeVirt 云主机提升到 500+\n",
                                            }
                                        ]
                                    }
                                },
                            },
                        ],
                    }
                ],
            },
        }
        self.get_calls: list[tuple[str, dict[str, Any]]] = []

    def get(self, url: str, **kwargs: Any) -> FakeResponse:
        self.get_calls.append((url, kwargs))
        if url == "https://www.xfchat.iflytek.com/lgw/csrf_token":
            self.cookies.values["lgw_csrf_token"] = "lgw-fixture"
            return FakeResponse({})
        if "/okrx/api/okr/owner/aggr_detail/" in url:
            return FakeResponse(self.payload)
        raise AssertionError(f"unexpected GET {url}")


def test_detect_okr_url_recognizes_okr_pages_only():
    assert detect_okr_url(OKR_URL) is True
    assert detect_okr_url("https://example.xfchat.iflytek.com/docx/doxfixture") is False


def test_read_okr_uses_lgw_csrf_and_okr_id_query_param():
    session = FakeSession()

    title, token, body, counts = read_okr(session, OKR_URL)  # type: ignore[arg-type]

    assert title == "OKR - Fixture Owner - 2026 年 7 月 - 9 月"
    assert token == "okr-fixture-200"
    assert counts == {"objectives": 1, "key_results": 2}
    assert "# OKR - Fixture Owner - 2026 年 7 月 - 9 月" in body
    assert "## O1 支撑计算平台规模化落地" in body
    assert "- KR1: SAE 生产应用数提升到 8000 _(progress: 20%)_" in body
    assert "- KR2: KubeVirt 云主机提升到 500+" in body

    urls = [call[0] for call in session.get_calls]
    assert urls[0] == "https://www.xfchat.iflytek.com/lgw/csrf_token"
    detail_url, detail_kwargs = session.get_calls[1]
    assert "/okrx/api/okr/owner/aggr_detail/" in detail_url
    assert "okr_id=okr-fixture-200" in detail_url
    assert "okrId=" not in detail_url
    assert detail_kwargs["headers"]["x-lgw-csrf-token"] == "lgw-fixture"


def test_read_okr_nonzero_response_does_not_expose_payload():
    session = FakeSession(
        {
            "code": 403,
            "message": "private failure",
            "okr_detail_data": {"objective_list": [{"name": "secret objective"}]},
        }
    )

    with pytest.raises(RuntimeError) as exc_info:
        read_okr(session, OKR_URL)  # type: ignore[arg-type]

    message = str(exc_info.value)
    assert message == "OKR aggr_detail failed with code 403."
    assert "private failure" not in message
    assert "secret objective" not in message


def test_read_okr_url_loads_cookies_and_returns_result(monkeypatch, tmp_path):
    cookies = tmp_path / "cookies.json"
    calls: dict[str, Any] = {}

    def fake_load_cookie_objects(path):
        calls["cookies_path"] = path
        return [{"name": "session", "value": "s"}]

    monkeypatch.setattr("ixf_toolbox.core.okr.reader.load_cookie_objects", fake_load_cookie_objects)

    def fake_build_session(cookie_objects):
        calls["cookie_objects"] = cookie_objects
        return FakeSession()

    monkeypatch.setattr("ixf_toolbox.core.okr.reader.build_session", fake_build_session)

    result = read_okr_url(OKR_URL, cookies_path=cookies)

    assert calls["cookies_path"] == cookies
    assert calls["cookie_objects"] == [{"name": "session", "value": "s"}]
    assert result["kind"] == "okr"
    assert result["token"] == "okr-fixture-200"
    assert result["counts"] == {"objectives": 1, "key_results": 2}
