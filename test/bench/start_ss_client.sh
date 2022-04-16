echo "start ss in tcp tun mode."

go-shadowsocks2 -c 'ss://AEAD_CHACHA20_POLY1305:your-password@[0.0.0.0]:8488' -tcptun :1090=0.0.0.0:5201 -verbose
