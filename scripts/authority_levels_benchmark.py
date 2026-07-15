#!/usr/bin/env python3
"""Deterministic authority-level benchmark for provider/model/pricing routing.

The benchmark scores the current code against a small, source-grounded contract for
making provider groups, providers, pricing, and model metadata explicit authority
surfaces. It does not hit network or databases; it only reads repository files.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def read_repo(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


FILES = {
    "model/provider_group.go": read_repo("model/provider_group.go"),
    "service/group.go": read_repo("service/group.go"),
    "controller/model.go": read_repo("controller/model.go"),
    "relay/helper/price.go": read_repo("relay/helper/price.go"),
    "service/quota.go": read_repo("service/quota.go"),
    "model/model_meta.go": read_repo("model/model_meta.go"),
    "model/pricing.go": read_repo("model/pricing.go"),
    "model/pricing_default.go": read_repo("model/pricing_default.go"),
    "setting/ratio_setting/model_ratio.go": read_repo("setting/ratio_setting/model_ratio.go"),
    "setting/ratio_setting/official_pricing.go": read_repo("setting/ratio_setting/official_pricing.go"),
    "setting/ratio_setting/model_metadata_pricing.go": read_repo("setting/ratio_setting/model_metadata_pricing.go"),
}


@dataclass(frozen=True)
class Contract:
    domain: str
    name: str
    passed: bool
    evidence: str


def contains(path: str, pattern: str) -> bool:
    return re.search(pattern, FILES[path], re.MULTILINE | re.DOTALL) is not None


def ordered(path: str, first: str, second: str) -> bool:
    text = FILES[path]
    first_index = text.find(first)
    second_index = text.find(second)
    return first_index >= 0 and second_index >= 0 and first_index < second_index


def target_text() -> str:
    return "\n".join(FILES.values())


provider_group = FILES["model/provider_group.go"]
service_group = FILES["service/group.go"]
controller_model = FILES["controller/model.go"]
price_helper = FILES["relay/helper/price.go"]
quota_service = FILES["service/quota.go"]
model_meta = FILES["model/model_meta.go"]
pricing = FILES["model/pricing.go"]
pricing_default = FILES["model/pricing_default.go"]
model_ratio = FILES["setting/ratio_setting/model_ratio.go"]

contracts = [
    Contract(
        "channel_groups",
        "provider_group_tables_define_routing_authority",
        all(token in provider_group for token in ("type ProviderGroup struct", "type ProviderGroupChannel struct", "type ProviderGroupAutoRule struct")),
        "model/provider_group.go defines provider_groups, provider_group_channels, provider_group_auto_rules",
    ),
    Contract(
        "channel_groups",
        "abilities_are_derived_from_provider_groups",
        "func RebuildAbilitiesFromProviderGroups" in provider_group and "func rebuildAbilitiesFromProviderGroups" in provider_group,
        "model/provider_group.go materializes abilities from enabled provider group memberships",
    ),
    Contract(
        "channel_groups",
        "auto_model_visibility_uses_all_provider_auto_rules",
        "GetProviderAutoModelGroups" in provider_group and "GetRequestAutoModelGroups" in service_group,
        "auto /v1/models uses route-neutral provider auto candidates",
    ),
    Contract(
        "channel_groups",
        "direct_model_list_checks_provider_family_gate",
        "ProviderGroupAccessError(c, group)" in controller_model,
        "controller/model.go checks direct provider groups before listing models",
    ),
    Contract(
        "pricing",
        "provider_group_usage_ratio_wins_in_text_preconsume",
        ordered("relay/helper/price.go", "model.ProviderGroupUsageRatio(relayInfo.UsingGroup)", "ratio_setting.GetGroupRatio(relayInfo.UsingGroup)"),
        "relay/helper/price.go checks ProviderGroupUsageRatio before legacy group ratios",
    ),
    Contract(
        "pricing",
        "provider_group_usage_ratio_wins_in_realtime_preconsume",
        "ProviderGroupUsageRatio" in quota_service,
        "service/quota.go should use provider group ratio authority for realtime quota paths",
    ),
    Contract(
        "model",
        "model_rows_carry_local_vs_official_source_marker",
        "PricingConfig string" in model_meta and "SyncOfficial  int" in model_meta,
        "model/model_meta.go carries pricing_config plus sync_official",
    ),
    Contract(
        "model",
        "authority_level_is_explicit_three_state_contract",
        re.search(r"Authority(Level|Rank)|SourceAuthority|AuthoritySource", target_text()) is not None,
        "expected a named three-level authority contract instead of booleans and naming conventions",
    ),
    Contract(
        "pricing",
        "model_metadata_pricing_overrides_official_pricing",
        ordered("setting/ratio_setting/model_ratio.go", "GetMetadataModelRatio(name)", "GetOfficialModelRatio(name)"),
        "model metadata pricing is resolved before official pricing",
    ),
    Contract(
        "pricing",
        "official_pricing_blocks_legacy_ratio_when_authoritative",
        "!OfficialPricingAuthoritative()" in model_ratio and "modelRatioMap.Get(name)" in model_ratio,
        "legacy ratio maps are skipped when official pricing is authoritative",
    ),
    Contract(
        "provider",
        "family_gate_is_data_driven_not_group_name_inferred",
        re.search(r"Required(Client)?Family|AccessPolicy|ClientFamily", provider_group) is not None,
        "provider family access should be stored as provider-group metadata, not inferred from claude-max/codex-pro names",
    ),
    Contract(
        "provider",
        "pricing_refresh_does_not_create_provider_rows",
        "getOrCreateVendor" not in pricing_default and ".Insert()" not in pricing_default,
        "pricing refresh should not create provider/vendor authority rows as a read-side effect",
    ),
    Contract(
        "pricing",
        "pricing_result_exposes_billing_mode_and_expression",
        "BillingMode" in pricing and "BillingExpr" in pricing and "usesTieredBilling" in pricing,
        "pricing API exposes tiered billing mode/expression for model metadata pricing",
    ),
]

failed = [contract for contract in contracts if not contract.passed]
passed = [contract for contract in contracts if contract.passed]
name_inferred_gates = len(re.findall(r"claude-max|codex-pro", service_group))
legacy_group_ratio_reads = len(re.findall(r"ratio_setting\.GetGroupRatio\(", price_helper + quota_service))
explicit_authority_terms = len(re.findall(r"Authority(Level|Rank)|SourceAuthority|AuthoritySource", target_text()))
read_side_effect_writes = len(re.findall(r"\.Insert\(\)", pricing_default))

print("authority benchmark: provider/channel group/pricing/model")
for contract in contracts:
    status = "PASS" if contract.passed else "FAIL"
    print(f"{status} {contract.domain}.{contract.name}: {contract.evidence}")

print(f"METRIC authority_gap_count={len(failed)}")
print(f"METRIC authority_contract_count={len(contracts)}")
print(f"METRIC authority_contract_pass_count={len(passed)}")
print(f"METRIC name_inferred_gate_count={name_inferred_gates}")
print(f"METRIC legacy_group_ratio_read_count={legacy_group_ratio_reads}")
print(f"METRIC explicit_authority_term_count={explicit_authority_terms}")
print(f"METRIC pricing_refresh_write_side_effect_count={read_side_effect_writes}")
print("ASI failed_contracts=" + (",".join(contract.name for contract in failed) if failed else "none"))
print("ASI benchmark=authority-level-static-contract-v1")
