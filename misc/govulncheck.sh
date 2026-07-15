#!/bin/bash

# Vulnerabilities to ignore (no fix available yet).
# Current as of 2026-07-08. Remove entries from this list once fixed versions are available.
IGNORED_VULNS=(
  "GO-2026-4887"
  "GO-2026-4883"
  "GO-2026-5617"
  "GO-2026-5668"
  "GO-2026-5746"
  "GO-2026-5064"
  "GO-2026-5338"
  "GO-2026-5622"
)

set +e
output=$(govulncheck ./... 2>/dev/null)
exit_code=$?
set -e

if [ $exit_code -eq 0 ]; then
  echo "No vulnerabilities found."
  exit 0
fi

# Extract unique vulnerability IDs from text output
found_vulns=$(echo "$output" | grep -oE 'GO-[0-9]+-[0-9]+' | sort -u)

remaining=()
for vuln in $found_vulns; do
  skip=false
  for ignored in "${IGNORED_VULNS[@]}"; do
    if [ "$vuln" = "$ignored" ]; then
      skip=true
      break
    fi
  done
  if [ "$skip" = "false" ]; then
    remaining+=("$vuln")
  fi
done

if [ ${#remaining[@]} -gt 0 ]; then
  echo "::error::Unignored vulnerabilities found:"
  printf '  %s\n' "${remaining[@]}"
  echo ""
  echo "Full govulncheck output:"
  echo "$output"
  exit 1
fi

echo "All found vulnerabilities are in the ignore list:"
printf '  %s\n' "${IGNORED_VULNS[@]}"
echo "::warning::Ignored vulnerabilities (no fix available): ${IGNORED_VULNS[*]}"
