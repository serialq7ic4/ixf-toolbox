import json
import importlib
from pathlib import Path

import pytest


FIXTURE = Path(__file__).parent / "fixtures" / "okr_write.json"
OKR_URL = "https://tenant.example.test/okr/user/example/?okrId=example-okr"


@pytest.fixture
def writer():
    try:
        return importlib.import_module("ixf_toolbox.core.okr.writer")
    except ModuleNotFoundError:
        class MissingWriter:
            def __getattr__(self, name):
                pytest.fail("missing native OKR writer module")

        return MissingWriter()


def test_parse_specs_reads_objective_and_krs(writer):
    specs = writer.parse_specs(FIXTURE)

    assert specs == [
        writer.ObjectiveSpec(
            "Improve platform reliability",
            [
                "Complete the first measurable reliability milestone.",
                "Complete the second measurable reliability milestone.",
                "Complete the third measurable reliability milestone.",
            ],
        )
    ]


def test_parse_specs_rejects_more_than_four_krs(writer, tmp_path):
    input_path = tmp_path / "okr.json"
    input_path.write_text(
        json.dumps(
            {
                "objectives": [
                    {
                        "objective": "Too broad",
                        "krs": ["KR1", "KR2", "KR3", "KR4", "KR5"],
                    }
                ]
            }
        ),
        encoding="utf-8",
    )

    with pytest.raises(ValueError, match="keep OKR scope realistic"):
        writer.parse_specs(input_path)


def test_base_url_is_derived_from_target_url(writer):
    assert writer.base_url_from_url(OKR_URL) == "https://tenant.example.test"


def test_draft_payload_uses_confirmed_wire_fields(writer):
    body = writer.draft_v2_body("version-1", "conn-1")
    params = writer.delete_params("version-1", "conn-1")

    assert body["draft_version"] == "version-1"
    assert body["conn_uuid"] == "conn-1"
    assert "token" in body
    assert params["draft_version"] == "version-1"
    assert params["conn_uuid"] == "conn-1"
    assert "draftVersion" not in body


def test_delta_doc_uses_zoned_editor_payload(writer):
    assert writer.delta_doc("new O3") == {
        "0": {
            "ops": [{"insert": "new O3\n"}],
            "zoneId": "0",
            "zoneType": "Z",
        }
    }


def test_publish_objective_uses_objective_endpoint(writer, monkeypatch):
    captured = {}

    def fake_call_with_version(*args, **kwargs):
        captured["args"] = args
        captured["kwargs"] = kwargs
        return {"code": 0, "data": {"draft_version": "next"}}

    monkeypatch.setattr(writer, "call_with_version", fake_call_with_version)

    writer.publish_objective(
        object(),
        "https://tenant.example.test",
        OKR_URL,
        "example-okr",
        "objective-3",
        "conn-1",
        delete_kr_ids=["old-kr"],
    )

    assert captured["args"][5] == "/okrx/api/draft_v2/publish/objective-3/"
    body = captured["kwargs"]["make_body"]("version-1", "conn-1")
    assert body["need_delete_kr_ids"] == ["old-kr"]
    assert body["auto_notify"] is False


def test_existing_target_enters_edit_state_before_mutation(writer, monkeypatch):
    calls = []
    existing = {
        "id": "objective-3",
        "objective": "Old O3",
        "krs": [{"id": "old-kr", "text": "Old KR"}],
    }
    spec = writer.ObjectiveSpec("New O3", ["New KR"])
    before = [
        {"id": "objective-1", "objective": "O1", "krs": []},
        {"id": "objective-2", "objective": "O2", "krs": []},
        existing,
    ]

    monkeypatch.setattr(
        writer,
        "enable_objective_draft",
        lambda *args, **kwargs: calls.append("enable"),
    )
    monkeypatch.setattr(
        writer,
        "update_objective_text",
        lambda *args, **kwargs: calls.append("objective"),
    )
    monkeypatch.setattr(
        writer,
        "replace_target_krs",
        lambda **kwargs: calls.append("krs") or ["old-kr"],
    )
    monkeypatch.setattr(
        writer,
        "publish_objective",
        lambda *args, **kwargs: calls.append("publish"),
    )
    monkeypatch.setattr(
        writer,
        "verify_target_and_preserved_neighbors",
        lambda **kwargs: {"objective": "New O3", "krs": [{"text": "New KR"}]},
    )

    result = writer.update_existing_target(
        session=object(),
        base_url="https://tenant.example.test",
        okr_url=OKR_URL,
        okr_id="example-okr",
        existing=existing,
        spec=spec,
        index=3,
        before_state=before,
        conn_uuid="conn-1",
    )

    assert calls == ["enable", "objective", "krs", "publish"]
    assert result["objective"] == "New O3"


def test_non_target_objectives_are_compared_by_position(writer):
    before = [
        {"id": "o1", "objective": "O1", "krs": [{"id": "k1", "text": "K1"}]},
        {"id": "o2", "objective": "O2", "krs": [{"id": "k2", "text": "K2"}]},
        {"id": "o3", "objective": "Old O3", "krs": []},
    ]
    after = [before[0], before[1], {"id": "o3", "objective": "New O3", "krs": []}]

    assert writer.non_target_objectives(before, 3) == writer.non_target_objectives(after, 3)
