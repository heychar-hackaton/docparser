#!/usr/bin/env bash
set -euo pipefail

echo "Installing pdftotext (Poppler) ..."

if command -v pdftotext >/dev/null 2>&1; then
  echo "pdftotext is already installed: $(command -v pdftotext)"
  exit 0
fi

OS="$(uname -s)"
LOWER_OS="$(echo "$OS" | tr '[:upper:]' '[:lower:]')"

if [[ "$LOWER_OS" == "darwin" ]]; then
  if command -v brew >/dev/null 2>&1; then
    echo "Using Homebrew to install poppler ..."
    brew update
    brew install poppler
  else
    echo "Homebrew is not installed. Install Homebrew first:"
    echo "/bin/bash -c \"$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
    exit 1
  fi
  exit 0
fi

# Linux and others
if [[ -f /etc/os-release ]]; then
  # shellcheck disable=SC1091
  . /etc/os-release
  ID_LOWER="${ID:-unknown}"
  ID_LIKE_LOWER="${ID_LIKE:-}"
else
  ID_LOWER="unknown"
  ID_LIKE_LOWER=""
fi

try_install() {
  local cmd="$1"; shift
  if command -v "$cmd" >/dev/null 2>&1; then
    echo "Using $cmd to install: $*"
    sudo "$cmd" "$@"
    return 0
  fi
  return 1
}

install_success=false

case "$ID_LOWER" in
  ubuntu|debian)
    sudo apt-get update
    sudo apt-get install -y poppler-utils
    install_success=true
    ;;
  fedora)
    sudo dnf install -y poppler-utils
    install_success=true
    ;;
  rhel|centos)
    if try_install dnf install -y poppler-utils; then install_success=true; fi
    if [[ $install_success == false ]] && try_install yum install -y poppler-utils; then install_success=true; fi
    ;;
  arch|archlinux)
    sudo pacman -Sy --noconfirm poppler
    install_success=true
    ;;
  alpine)
    sudo apk add --no-cache poppler-utils
    install_success=true
    ;;
  opensuse*|sles)
    sudo zypper refresh
    sudo zypper install -y poppler-tools
    install_success=true
    ;;
esac

if [[ $install_success == false ]]; then
  # Fallback: try best-effort by probing common managers
  if command -v apt-get >/dev/null 2>&1; then
    sudo apt-get update && sudo apt-get install -y poppler-utils && install_success=true
  elif command -v dnf >/dev/null 2>&1; then
    sudo dnf install -y poppler-utils && install_success=true
  elif command -v yum >/dev/null 2>&1; then
    sudo yum install -y poppler-utils && install_success=true
  elif command -v pacman >/dev/null 2>&1; then
    sudo pacman -Sy --noconfirm poppler && install_success=true
  elif command -v apk >/dev/null 2>&1; then
    sudo apk add --no-cache poppler-utils && install_success=true
  elif command -v zypper >/dev/null 2>&1; then
    sudo zypper refresh && sudo zypper install -y poppler-tools && install_success=true
  fi
fi

if [[ $install_success == false ]]; then
  echo "Could not automatically install pdftotext. Please install Poppler manually for your distro."
  exit 1
fi

if ! command -v pdftotext >/dev/null 2>&1; then
  echo "Installation finished but pdftotext not found. Ensure your PATH is updated."
  exit 1
fi

echo "pdftotext installed: $(command -v pdftotext)"

