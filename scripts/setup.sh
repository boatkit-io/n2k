#!/usr/bin/env bash
# Sets up all dependencies. LIKE A BOSS.

set -e

FIRST_RUN=false
if [[ -z "$ASDF_DIR" ]]; then
  FIRST_RUN=true
fi

if ! command -v asdf >/dev/null 2>&1; then
  source ./scripts/setup_asdf.sh
fi

# gh -- skip when in docker build
if [[ -z "${INCONTAINER}" ]] && ! command -v gh >/dev/null; then
  echo "Installing gh ..."
  curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list >/dev/null
  sudo apt update -y
  sudo apt install -y gh
fi

if [[ "$FIRST_RUN" == "true" ]]; then
  echo ""
  echo "Please restart your shell. This will only be needed once."
  echo ""
  sleep infinity
fi
