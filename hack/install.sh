#!/usr/bin/env bash

set -e
###
# SCRIPT CONFIGURATION
###

# Basename of this script
SCRIPT_NAME="$(basename "$0")"

# Command line arguments of this script
SCRIPT_ARGS=("$@")

# Path for installing executable
EXECUTABLE_INSTALL_PATH="/usr/local/bin/ehco"

# Paths to install systemd files
SYSTEMD_SERVICES_DIR="/etc/systemd/system"

# URLs of GitHub
REPO_URL="https://github.com/Ehco1996/Ehco"

# curl command line flags.
# To using a proxy, please specify ALL_PROXY in the environ variable, such like:
# export ALL_PROXY=socks5h://192.0.2.1:1080
CURL_FLAGS=(-L -f -q --retry 5 --retry-delay 10 --retry-max-time 60)

###
# AUTO DETECTED GLOBAL VARIABLE
###

# Package manager
PACKAGE_MANAGEMENT_INSTALL="${PACKAGE_MANAGEMENT_INSTALL:-}"

# Operating System of current machine, supported: linux
OPERATING_SYSTEM="${OPERATING_SYSTEM:-}"

# Architecture of current machine, supported: 386, amd64, arm, arm64, mipsle, s390x
ARCHITECTURE="${ARCHITECTURE:-}"

###
# ARGUMENTS
###

# Supported operation: install, remove, check_update, help
OPERATION=

# User specified version to install when empty latest version will be installed
VERSION=

# support file path for configuration or API endpoint
API_OR_CONFIG_PATH=

# help function

function print_error_msg() {
    local _msg="$1"
    echo -e "\033[31m$_msg\033[0m"
}

function print_help() {
    echo "Usage: $SCRIPT_NAME [options]"
    echo
    echo "Options:"
    echo "  -h, --help          Show this help message and exit."
    echo "  -v, --version       Specify the version to install."
    echo "  -i, --install       Install the Ehco."
    echo "  -r, --remove        Remove the Ehco."
    echo "  -u, --check-update  Check if an update is available."
}

function parse_arguments() {
    while [[ "$#" -gt 0 ]]; do
        case "$1" in
        -h | --help)
            print_help
            exit 0
            ;;
        -v | --version)
            VERSION="$2"
            shift
            ;;
        -i | --install)
            OPERATION="install"
            ;;
        -r | --remove)
            OPERATION="remove"
            ;;
        -u | --check-update)
            OPERATION="check_update"
            ;;
        *)
            print_error_msg "Unknown argument: $1"
            ;;
        esac
        shift
    done
    if [[ -z "$OPERATION" ]]; then
        print_error_msg "Operation not specified."
    fi
}

function get_release_assets_urls() {
    local _version="$1"
    local api_url="https://api.github.com/repos/Ehco1996/Ehco/releases/tags/${_version}"
    echo "$api_url"
    curl -sSL "$api_url" | jq -r '.assets[] | .browser_download_url'
}

function download_release_asset() {
    local _assets_json=\$1

    # Detect host architecture
    arch=$(uname -m)
    case $arch in
    x86_64)
        target_arch="amd64"
        ;;
    aarch64)
        target_arch="arm64"
        ;;
    *)
        echo "Unsupported architecture: $arch"
        return 1
        ;;
    esac

    # Extract the download URL for the target architecture using jq
    download_url=$(echo "$_assets_json" | jq -r --arg target_arch "$target_arch" \
        '.assets[] | select(.name | contains("linux_" + $target_arch)) | .browser_download_url')
    echo "Download URL for architecture $target_arch: $download_url"

    if [ -z "$download_url" ]; then
        echo "Download URL for architecture $target_arch not found."
        return 1
    fi
    # Download the file
    echo "Downloading $download_url..."
    curl -L "$download_url" -o "release_$target_arch"
}

function perform_install() {
    local _version=$VERSION
    # if version is not specified, set it to latest
    if [[ -z "$_version" ]]; then
        echo "not specified version, will install nightly version"
        _version="v0.0.0-nightly"
    fi
}

###
# Entrypoint
###
function main() {
    parse_arguments "$@"
    case "$OPERATION" in
    "install")
        perform_install
        ;;
    "remove")
        # perform_remove
        ;;
    "check_update")
        # perform_check_update
        ;;
    *)
        print_error_msg "Unknown operation: '$OPERATION'."
        ;;
    esac
}

# main "$@"

perform_install
