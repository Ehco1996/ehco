# ehco

ehco is a network relay tool and a typo :)

## 主要功能

* tcp/udp relay
* tcp/udp relay over wss
* 从配置文件启动
* 从远程启动
* benchmark


## TODO

* 热重载配置


## 使用说明

参考Benchmark


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
