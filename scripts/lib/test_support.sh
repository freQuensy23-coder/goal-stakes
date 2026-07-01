#!/usr/bin/env bash

ensure_forge_std() {
  local root="$1"
  local dep="$root/web3/lib/forge-std/src/Test.sol"

  if [[ -f "$dep" ]]; then
    return 0
  fi

  if command -v git >/dev/null 2>&1 && [[ -f "$root/.gitmodules" ]]; then
    git -C "$root" submodule update --init --recursive -- web3/lib/forge-std
  fi

  if [[ ! -f "$dep" ]]; then
    echo "missing Foundry dependency: web3/lib/forge-std" >&2
    echo "run: git submodule update --init --recursive web3/lib/forge-std" >&2
    exit 1
  fi
}
