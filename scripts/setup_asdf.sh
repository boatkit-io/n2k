#!/usr/bin/env bash
# Installs and configures asdf for all shells on the current system
ASDF_DIR="${ASDF_DIR:-"$HOME/.asdf"}"

shellrcs=("$HOME/.zshrc" "$HOME/.bashrc")
marker="###Marker(asdf):"

if [[ ! -e "$HOME/.asdf" ]]; then
  echo "[asdf] Installing asdf to $ASDF_DIR"
  git clone https://github.com/asdf-vm/asdf.git "$ASDF_DIR" --branch v0.10.0
else
  echo "[asdf] Updating asdf"
  if ! command -v asdf >/dev/null; then
    # shellcheck disable=SC1091 # Why: We don't control these
    . "$HOME/.asdf/asdf.sh"
    # shellcheck disable=SC1091 # Why: We don't control these
    . "$HOME/.asdf/completions/asdf.bash"
  fi

  asdf update
fi

for shellrc in "${shellrcs[@]}"; do
  if [[ ! -e $shellrc ]]; then
    echo "[asdf] Skipping shellrc '$shellrc', doesn't exist"
  fi

  # Only ever write to this shellrc once
  if grep "$marker" "$shellrc" >/dev/null 2>&1; then
    continue
  fi

  # shellcheck disable=SC2016 # Why: We want these to expand at shell runtime, not now.
  {
    echo ""
    echo "$marker Start asdf import block"
    echo '. "$HOME/.asdf/asdf.sh"'
    echo '. "$HOME/.asdf/completions/asdf.bash"'
    echo "###EndMarker(asdf): End asdf import block"
    echo ""
  } >>"$shellrc"
done

# shellcheck disable=SC1091 # Why: We don't control these
. "$HOME/.asdf/asdf.sh"
# shellcheck disable=SC1091 # Why: We don't control these
. "$HOME/.asdf/completions/asdf.bash"

echo "[asdf] Installing plugins"
while IFS="" read -r p || [[ -n "$p" ]]; do
  plugin="$(awk '{ print $1 }' <<<"$p")"
  if ! asdf plugin list | grep -q "$plugin"; then
    asdf plugin add "$plugin"
  fi
done <.tool-versions

echo "[asdf] Installing languages"
asdf install
