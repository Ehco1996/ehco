#!/usr/bin/env bash

set -e
###
# SCRIPT CONFIGURATION
###

# Basename of this script
SCRIPT_NAME="$(basename "$0")"

# Path for installing executable
EXECUTABLE_INSTALL_PATH="/usr/local/bin/ehco"

# Paths to install systemd files
SYSTEMD_SERVICES_DIR="/etc/systemd/system"

# curl command line flags.
# To using a proxy, please specify ALL_PROXY in the environ variable, such like:
# export ALL_PROXY=socks5h://192.0.2.1:1080
CURL_FLAGS=(-L -f -q --retry 5 --retry-delay 10 --retry-max-time 60)

###
# ARGUMENTS
###

# Supported operation: install, remove, check_update, help
OPERATION=

# User specified version to install when empty latest version will be installed
VERSION=

# support file path for configuration or API endpoint
API_OR_CONFIG_PATH=

# auto detect target arch
TARGET_ARCH=

###
# HELPER FUNCTIONS
###

detect_arch() {
    # 检查操作系统
    local os=$(uname -s)
    if [ "$os" != "Linux" ]; then
        echo "This script only supports Linux for now."
        exit 1
    fi

    # 检查架构并设置 target_arch
    local arch=$(uname -m)
    case $arch in
    x86_64)
        echo "Current arch is linux_amd64"
        TARGET_ARCH="linux_amd64"
        ;;
    aarch64)
        echo "Current arch is linux_arm64"
        TARGET_ARCH="linux_arm64"
        ;;
    *)
        echo "Unsupported architecture: $arch" >&2
        ;;
    esac
}

function print_error_msg() {
    local _msg="$1"
    echo -e "\033[31m$_msg\033[0m"
}

function print_warning_msg() {
    local _msg="$1"
    echo -e "\033[33m$_msg\033[0m"
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

function set_default_version() {
    # if version is not specified, set it to latest
    if [[ -z "$VERSION" ]]; then
        print_warning_msg "Version not specified. Using **nightly** as the default version."
        VERSION="v0.0.0-nightly"
    fi
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
    set_default_version
}

# TODO check the checksum and current bin file, if the same, skip download
function download_bin() {
    local api_url="https://api.github.com/repos/Ehco1996/ehco/releases/tags/$VERSION"
    local _assets_json
    _assets_json=$(curl -s "${CURL_FLAGS[@]}" "$api_url")

    # Extract the download URL for the target architecture using jq
    download_url=$(echo "$_assets_json" | jq -r --arg TARGET_ARCH "$TARGET_ARCH" '.assets[] | select(.name | contains("ehco_" + $TARGET_ARCH)) | .browser_download_url')
    if [ -z "$download_url" ]; then
        echo "Download URL for architecture $TARGET_ARCH not found."
        return 1
    fi
    # Download the file
    curl "${CURL_FLAGS[@]}" -o "$EXECUTABLE_INSTALL_PATH" "$download_url"
    echo "Downloaded and Install **ehco** to $EXECUTABLE_INSTALL_PATH"
    chmod +x "$EXECUTABLE_INSTALL_PATH"
}

function install_systemd_service() {
    local _service_name="ehco.service"
    local _service_path="$SYSTEMD_SERVICES_DIR/$_service_name"
}

function perform_install() {
    download_bin
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

parse_arguments "$@"
detect_arch
perform_install
