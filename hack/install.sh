#!/bin/bash

# 检查 EHCO_CONFIG_PATH_OR_URL 环境变量是否已设置
if [ -z "$EHCO_CONFIG_PATH_OR_URL" ]; then
    echo "EHCO_CONFIG_PATH_OR_URL environment variable is not set."
    echo "Usage: curl <script_url> | EHCO_CONFIG_PATH_OR_URL=<config_path_or_url> bash"
    exit 1
fi

# 定义下载和安装 ehco 的函数
install_ehco() {
    # 获取最新的 ehco nightly 版本的下载链接
    DOWNLOAD_URL=$(curl -s https://api.github.com/repos/Ehco1996/ehco/releases/tags/nightly | grep browser_download_url | grep linux_amd64 | head -n 1 | cut -d '"' -f 4)

    if [ -z "$DOWNLOAD_URL" ]; then
        echo "Failed to find the download URL for ehco."
        exit 1
    fi

    # 下载最新版本
    curl -L "$DOWNLOAD_URL" -o ehco.tar.gz

    # 解压
    tar -xzf ehco.tar.gz

    # 假设解压后的二进制文件名为 ehco，将其移动到 /usr/local/bin
    mv ehco /usr/local/bin/ehco

    # 清理下载的文件
    rm ehco.tar.gz
}

# 创建 systemd 服务文件
create_systemd_service() {
    cat <<EOF >/etc/systemd/system/ehco.service
[Unit]
Description=Ehco Service
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/ehco -c $EHCO_CONFIG_PATH_OR_URL
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF
}

# 主执行流程
install_ehco
create_systemd_service

# 重新加载 systemd，启动并启用 ehco 服务
systemctl daemon-reload
systemctl start ehco.service
systemctl enable ehco.service

echo "Ehco has been installed and started successfully."
