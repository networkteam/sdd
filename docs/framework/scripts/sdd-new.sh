#!/usr/bin/env bash
set -euo pipefail

# Create a new SDD graph entry (signal, decision, or action)
#
# Usage:
#   sdd-new.sh <type> <layer> [--refs ref1,ref2] [--participants p1,p2] [--confidence high|medium|low] [description...]
#
# Examples:
#   sdd-new.sh s stg "customers asking about shipping beans"
#   sdd-new.sh d cpt --refs 20260405-084500-s-stg-j3n --confidence medium "subscription with sharing built in"
#   sdd-new.sh a tac --refs 20260407-145000-d-tac-k8p "built prototype and deployed to staging"

GRAPH_DIR="$(git rev-parse --show-toplevel)/docs/framework/graph"

# Validate type
valid_types="d s a"
valid_type_names="decision signal action"
type_arg="${1:-}"
case "$type_arg" in
  d|decision)  type_abbr="d"; type_full="decision" ;;
  s|signal)    type_abbr="s"; type_full="signal" ;;
  a|action)    type_abbr="a"; type_full="action" ;;
  *)
    echo "Error: type must be one of: d (decision), s (signal), a (action)" >&2
    echo "Usage: sdd-new.sh <type> <layer> [--refs ref1,ref2] [--participants p1,p2] [--confidence high|medium|low] [description...]" >&2
    exit 1
    ;;
esac
shift

# Validate layer
layer_arg="${1:-}"
case "$layer_arg" in
  stg|strategic)    layer_abbr="stg"; layer_full="strategic" ;;
  cpt|conceptual)   layer_abbr="cpt"; layer_full="conceptual" ;;
  tac|tactical)     layer_abbr="tac"; layer_full="tactical" ;;
  ops|operational)  layer_abbr="ops"; layer_full="operational" ;;
  prc|process)      layer_abbr="prc"; layer_full="process" ;;
  *)
    echo "Error: layer must be one of: stg (strategic), cpt (conceptual), tac (tactical), ops (operational), prc (process)" >&2
    exit 1
    ;;
esac
shift

# Parse optional flags
refs=""
participants=""
confidence=""
description_parts=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --refs)
      refs="$2"
      shift 2
      ;;
    --participants)
      participants="$2"
      shift 2
      ;;
    --confidence)
      confidence="$2"
      shift 2
      ;;
    *)
      description_parts+=("$1")
      shift
      ;;
  esac
done

description="${description_parts[*]:-}"

# Generate ID: type-layer-YYYYMMDD-HHmmss-xxx
timestamp=$(date +%Y%m%d-%H%M%S)
random_suffix=$(head -c 100 /dev/urandom | LC_ALL=C tr -dc 'a-z0-9' | head -c3)
id="${timestamp}-${type_abbr}-${layer_abbr}-${random_suffix}"
filename="${id}.md"
filepath="${GRAPH_DIR}/${filename}"

# Build frontmatter
{
  echo "---"
  echo "type: ${type_full}"
  echo "layer: ${layer_full}"
  if [[ -n "$refs" ]]; then
    # Format refs as YAML list
    echo "refs:"
    IFS=',' read -ra ref_array <<< "$refs"
    for ref in "${ref_array[@]}"; do
      # Strip .md if present, add it back for consistency
      ref="${ref%.md}"
      echo "  - ${ref}"
    done
  fi
  if [[ -n "$participants" ]]; then
    echo "participants:"
    IFS=',' read -ra part_array <<< "$participants"
    for p in "${part_array[@]}"; do
      echo "  - ${p}"
    done
  fi
  if [[ -n "$confidence" ]]; then
    echo "confidence: ${confidence}"
  fi
  echo "---"
  echo ""
  if [[ -n "$description" ]]; then
    echo "$description"
  else
    echo "[TODO: describe this ${type_full}]"
  fi
} > "$filepath"

echo "$filename"
echo "  → ${filepath}"
