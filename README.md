# ehco

[![Go Report Card](https://goreportcard.com/badge/github.com/Ehco1996/ehco)](https://goreportcard.com/report/github.com/Ehco1996/ehco)
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/Ehco1996/ehco)

ehco is a network relay tool and a typo :)

## 主要功能

* tcp/udp relay
* tcp/(udp暂时不支持) relay over wss
* 从配置文件启动
* 从远程启动
* benchmark


## TODO

* 热重载配置


## 使用说明

使用隧道需要至少两条主机,并且在两台主机上都安装了ehco
> ehco的可执行文件可以从项目的[release](https://github.com/Ehco1996/ehco/releases)页面下载

* 中转机器 A 假设机器A的IP是 1.1.1.1
* 落地机器 B 假设机器B的IP是 2.2.2.2 并且落地机器B的5555端口跑着一个SS/v2ray服务


### 案例一 不用隧道直接通过中转机器中转用户流量

直接在中转机器A上输入: `ehco  -l 0.0.0.0:1234 -r 2.2.2.2:5555`
> 该命令表示将所有从中转机器A的1234端口进入的流量直接转发到落地机器B的5555端口

用户即可通过 中转机器A的1234端口访问到落地机器B的5555端口的SS/v2ray服务了

### 案例二 用mwss隧道中转用户流量

在落地机器B上输入: `ehco  -l 0.0.0.0:443 -lt mwss -r 127.0.0.1:5555`
> 该命令表示将所有从落地机器B的443端口进入的wss流量解密后转发到落地机器B的5555端口

在中转机器A上输入: `ehco  -l 0.0.0.0:1234 -r 2.2.2.2:443 -tt mwss`
> 该命令表示将所有从A的1234端口进入的流量通过wss加密后转发到落地机器B的443端口

用户即可通过 中转机器A的1234端口访问到落地机器B的5555端口的SS/v2ray服务了

## Benchmark

iperf:


```sh
# run iperf server on 5201
iperf3 -s

# 直接转发
# run relay server listen 1234 to 9001 (raw)
go run cmd/main.go -l 0.0.0.0:1234 -r 0.0.0.0:5201

# 直接转发END


# 通过wss隧道转发
# listen 1234 relay over wss to 1236
go run cmd/main.go -l 0.0.0.0:1235  -r wss://0.0.0.0:1236 -tt wss

# listen 1236 through wss relay to 5201
go run cmd/main.go -l 0.0.0.0:1236 -lt wss -r 0.0.0.0:5201
# 通过wss隧道转发END


# 通过mwss隧道转发 和wss相比 速度会慢，但是能减少延迟
# listen 1237 relay over mwss to 1238
go run cmd/main.go -l 0.0.0.0:1237  -r wss://0.0.0.0:1238 -tt mwss

# listen 1238 through mwss relay to 5201
go run cmd/main.go -l 0.0.0.0:1238 -lt mwss -r 0.0.0.0:5201
# 通过mwss隧道转发END

# run through file
go run cmd/main.go -c config.json

# benchmark tcp
iperf3 -c 0.0.0.0 -p 1234

# benchmark tcp through wss
iperf3 -c 0.0.0.0 -p 1235

# benchmark upd
iperf3 -c 0.0.0.0 -p 1234 -u -b 1G --length 1024

```

| iperf | raw | relay(raw) | relay(wss) | relay(mwss)|
| ---- | ----  | ---- | ---- | ---- |
| tcp  | 62.6 Gbits/sec | 23.9 Gbits/sec | 3.10 Gbits/sec | 1.77 Gbits/sec |
| udp  | 2.2 Gbits/sec | 2.2 Gbits/sec | xx  | xx |
