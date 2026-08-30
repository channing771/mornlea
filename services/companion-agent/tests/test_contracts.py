from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest
from pydantic import TypeAdapter, ValidationError

from mornlea_companion_agent.domain import CONTRACT_ADAPTERS, adapter_for
from mornlea_companion_agent.domain.http_v1 import PlanResponse
from mornlea_companion_agent.domain.mcp_v1 import (
    ACCEPTED_MINE_SEMANTICS,
    Plan,
    QueryTerrainResult,
)

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
CONTRACT_ROOT = REPOSITORY_ROOT / "contracts/companion-agent"


def _load(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


@pytest.mark.parametrize("contract", ["http-v1", "mcp-v1"])
def test_registry_covers_every_shared_schema_definition(contract: str) -> None:
    schema = _load(CONTRACT_ROOT / contract / "schema.json")
    assert set(CONTRACT_ADAPTERS[contract]) == set(schema["$defs"])


@pytest.mark.parametrize("contract", ["http-v1", "mcp-v1"])
def test_all_shared_valid_goldens_validate_without_copying_payloads(contract: str) -> None:
    fixture = _load(CONTRACT_ROOT / contract / "golden/valid.json")
    for case in fixture["cases"]:
        adapter = adapter_for(contract, case["schema"])
        adapter.validate_python(case["value"], context=case.get("context"))


@pytest.mark.parametrize("contract", ["http-v1", "mcp-v1"])
def test_all_shared_invalid_goldens_are_rejected(contract: str) -> None:
    fixture = _load(CONTRACT_ROOT / contract / "golden/invalid.json")
    accepted: list[str] = []
    for case in fixture["cases"]:
        adapter = adapter_for(contract, case["schema"])
        try:
            adapter.validate_python(case["value"], context=case.get("context"))
        except ValidationError:
            continue
        accepted.append(case["name"])
    assert accepted == []


def test_http_plan_reuses_the_mcp_plan_model() -> None:
    assert PlanResponse.model_fields["plan"].annotation is Plan


@pytest.mark.parametrize(
    ("schema", "value"),
    [
        ("uuid_v4", "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"),
        ("uuid_v4", "11111111-1111-1111-8111-111111111111"),
        ("sha256", "A" * 64),
        ("uint64", True),
        ("uint64", -1),
        ("uint64", 1 << 64),
    ],
)
def test_canonical_identity_digest_and_uint64_rules(schema: str, value: object) -> None:
    with pytest.raises(ValidationError):
        adapter_for("mcp-v1", schema).validate_python(value)


def test_plan_steps_are_exclusive_and_follow_is_last() -> None:
    plan_adapter: TypeAdapter[Plan] = adapter_for("mcp-v1", "plan")
    with pytest.raises(ValidationError):
        plan_adapter.validate_python(
            {
                "summary": "invalid overlap",
                "steps": [
                    {"kind": "follow", "player_id": "11111111-1111-4111-8111-111111111111", "x": 1}
                ],
            }
        )
    with pytest.raises(ValidationError):
        plan_adapter.validate_python(
            {
                "summary": "invalid ordering",
                "steps": [
                    {"kind": "follow", "player_id": "11111111-1111-4111-8111-111111111111"},
                    {"kind": "go_to", "x": 0, "y": 64, "z": 0},
                ],
            }
        )


def test_terrain_output_requires_call_input_positions_and_order() -> None:
    payload = {
        "terrain": [{"position": {"x": 1, "y": 64, "z": 2}, "height": 63, "block_name": "air"}]
    }
    context = {"positions": [{"x": 1, "y": 64, "z": 2}]}
    result = QueryTerrainResult.model_validate(payload, context=context)
    assert result.terrain[0].position.x == 1
    with pytest.raises(ValidationError):
        QueryTerrainResult.model_validate(payload, context={"positions": []})
    with pytest.raises(ValidationError):
        QueryTerrainResult.model_validate(payload)


def test_find_visible_blocks_output_is_strictly_coordinate_sorted() -> None:
    adapter = adapter_for("mcp-v1", "find_visible_blocks_result")
    with pytest.raises(ValidationError):
        adapter.validate_python(
            {
                "matches": [
                    {
                        "position": {"x": 2, "y": 64, "z": 0},
                        "block_name": "stone",
                        "drop_item": "stone",
                    },
                    {
                        "position": {"x": 1, "y": 64, "z": 0},
                        "block_name": "stone",
                        "drop_item": "stone",
                    },
                ]
            }
        )


def test_dialogue_terminal_matrix_and_memory_union_are_strict() -> None:
    terminal = adapter_for("http-v1", "dialogue_terminal_request")
    nonterminal_response = adapter_for("http-v1", "dialogue_nonterminal_response")
    zero_memory = adapter_for("http-v1", "memory_state_zero")
    with pytest.raises(ValidationError):
        terminal.validate_python({"terminal": True, "fact_node": {"kind": "idle"}})
    with pytest.raises(ValidationError):
        nonterminal_response.validate_python({"memory_proposal": {}})
    with pytest.raises(ValidationError):
        zero_memory.validate_python({"revision": 0, "operation_id": None, "summary": "not-empty"})


def test_mine_golden_only_checks_shared_classification_not_go_authority() -> None:
    manifest = _load(CONTRACT_ROOT / "mcp-v1/manifest.json")
    fixture = _load(CONTRACT_ROOT / "mcp-v1/golden/mine-validation.json")
    assert set(ACCEPTED_MINE_SEMANTICS) == set(manifest["mine_semantics"]["accepted"])
    assert {case["mine_semantics"] for case in fixture["cases"] if case["accepted"]} == set(
        ACCEPTED_MINE_SEMANTICS
    )
    assert {case["block_symbol"] for case in fixture["cases"] if case["accepted"]} >= {
        "ChestID",
        "FurnaceID",
    }
    assert all(
        case["accepted"] == (case["mine_semantics"] in ACCEPTED_MINE_SEMANTICS)
        for case in fixture["cases"]
    )
