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
CURL_FLAGS=(-L --retry 5 --retry-delay 10 --retry-max-time 60)

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

# use cloudflare proxy to download
USE_CF_PROXY=

###
# HELPER FUNCTIONS
###

function gen_ehco_systemd_service {
    cat <<EOF
[Unit]
Description=Ehco Service
After=network.target

[Service]
Type=simple
LimitNOFILE=65535
ExecStart=$EXECUTABLE_INSTALL_PATH -c "$API_OR_CONFIG_PATH"
WorkingDirectory=~
NoNewPrivileges=true
Restart=always

[Install]
WantedBy=multi-user.target
EOF
}

function _check_install_required() {
    if [[ -z "$API_OR_CONFIG_PATH" ]]; then
        _print_error_msg "Flag for config is required. please use --config to specify the configuration file path or api endpoint."
        exit 1
    fi

    # check jq and curl
    if ! command -v jq &>/dev/null; then
        _print_error_msg "jq is required to parse JSON data. please use apt/yum to install jq."
        exit 1
    fi

    if ! command -v curl &>/dev/null; then
        _print_error_msg "curl is required to download files. please use apt/yum to install curl."
        exit 1
    fi
}

function _detect_arch() {
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
        TARGET_ARCH="linux_amd64"
        ;;
    aarch64)
        TARGET_ARCH="linux_arm64"
        ;;
    *)
        echo "Unsupported architecture: $arch" >&2
        ;;
    esac
    _print_warning_msg "Detected architecture: $TARGET_ARCH"
}

function _print_error_msg() {
    local _msg="$1"
    echo -e "\033[31m$_msg\033[0m"
}

function _print_warning_msg() {
    local _msg="$1"
    echo -e "\033[33m$_msg\033[0m"
}

function _set_default_version() {
    # if version is not specified, set it to latest
    if [[ -z "$VERSION" ]]; then
        _print_warning_msg "Version not specified. Using **nightly** as the default version."
        VERSION="v0.0.0-nightly"
    fi
}

# TODO check the checksum and current bin file, if the same, skip download
function _download_bin() {
    printf "Downloading Ehco version: %s\n" "$VERSION"
    local api_url="https://api.github.com/repos/Ehco1996/ehco/releases/tags/$VERSION"
    local _assets_json
    _assets_json=$(curl "${CURL_FLAGS[@]}" "$api_url")

    # Extract the download URL for the target architecture using jq
    download_url=$(echo "$_assets_json" | jq -r --arg TARGET_ARCH "$TARGET_ARCH" '.assets[] | select(.name | contains("ehco_" + $TARGET_ARCH)) | .browser_download_url')
    if [ -z "$download_url" ]; then
        echo "Download URL for architecture $TARGET_ARCH not found."
        return 1
    fi

    # replace host to `release.ehco-relay.cc` to use cf-proxy to download
    if (( "$USE_CF_PROXY" == "true" )); then
        download_url=$(echo "$download_url" | sed 's|https://github.com|https://release.ehco-relay.cc|')
    fi

    echo "Download URL: $download_url"

    # Download the file
    curl "${CURL_FLAGS[@]}" -o "$EXECUTABLE_INSTALL_PATH" "$download_url"
    echo "Downloaded and Install **ehco** to $EXECUTABLE_INSTALL_PATH"
    chmod +x "$EXECUTABLE_INSTALL_PATH"
}

function _update_bin() {
    rm -f "$EXECUTABLE_INSTALL_PATH"
    _download_bin
}

function _install_systemd_service() {
    local _service_name="ehco.service"
    local _service_path="$SYSTEMD_SERVICES_DIR/$_service_name"
    gen_ehco_systemd_service >"$_service_path"
    systemctl daemon-reload
    systemctl enable "$_service_name"
    systemctl start "$_service_name"
}

function _reload_systemd_service() {
    systemctl daemon-reload
    systemctl restart ehco.service
}

function _remove_systemd_service_and_delete_bin() {
    local _service_name="ehco.service"
    local _service_path="$SYSTEMD_SERVICES_DIR/$_service_name"
    systemctl stop "$_service_name"
    systemctl disable "$_service_name"
    systemctl daemon-reload

    rm -f "$_service_path"
    rm -f "$EXECUTABLE_INSTALL_PATH"
}

function _check_systemd_service() {
    local _service_name="ehco.service"
    local _service_path="$SYSTEMD_SERVICES_DIR/$_service_name"
    if [ ! -f "$_service_path" ]; then
        _print_error_msg "Ehco service not found. please install it first."
        exit 1
    fi
}

function print_help() {
    echo "Usage: $SCRIPT_NAME [options]"
    echo
    echo "Options:"
    echo "  -h, --help          Show this help message and exit."
    echo "  -v, --version       Specify the version to install."
    echo "  -i, --install       Install the Ehco."
    echo "  -c, --config         Specify the configuration file path or api endpoint."
    echo "  -r, --remove        Remove the Ehco."
    echo "  -u, --check-update  Check And Update if an update is available."
    echo "  --cf-proxy          Use cloudflare proxy to download/update ehco bin."
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
        -c | --config)
            API_OR_CONFIG_PATH="$2"
            shift
            ;;
        -r | --remove)
            OPERATION="remove"
            ;;
        -u | --check-update)
            OPERATION="check-update"
            ;;
        --cf-proxy)
            USE_CF_PROXY="true"
            ;;
        *)
            _print_error_msg "Unknown argument: $1"
            exit 1
            ;;
        esac
        shift
    done
    if [[ -z "$OPERATION" ]]; then
        _print_error_msg "Operation not specified."
        print_help
        exit 1
    fi
}

function perform_install() {
    _check_install_required
    _set_default_version
    _detect_arch

    _download_bin
    _install_systemd_service
    _print_warning_msg "Ehco has been installed."
}

function perform_remove() {
    _remove_systemd_service_and_delete_bin
    _print_warning_msg "Ehco has been removed."
}

function perform_check_update() {
    _check_systemd_service
    _set_default_version
    _detect_arch

    _update_bin
    _reload_systemd_service
    _print_warning_msg "Ehco has been Updated."
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
        perform_remove
        ;;
    "check-update")
        perform_check_update
        ;;
    *)
        _print_error_msg "Unknown operation: '$OPERATION'."
        ;;
    esac
}

main "$@"
