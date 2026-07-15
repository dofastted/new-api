#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

python3 scripts/authority_levels_benchmark.py

go test ./model ./service ./controller -run 'ProviderGroup|GetRequestAutoGroup|ListModels|ModelPricingConfig|OfficialPricing' -count=1
