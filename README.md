# ehco
ehco is a echo proxy and a typo

## 主要功能

* tcp/udp relay
* 从配置文件启动
* 从远程启动

## TODO

* 热重载配置
* benchmark


## BenchMark

iperf:


```sh
# run iperf server on 9001
iperf3 -s -p 9001

# run relay server listen 1234 to 9001
go run cmd/main.go -l 0.0.0:1234 -r 0.0.0.0:9001

# benchmark tcp
iperf3 -c 0.0.0.0 -p 1234

# benchmark upd
iperf3 -c 0.0.0.0 -p 1234 -u -b 1G


```

| iperf | raw | relay(ehco_row) |
| ---- | ----  | ---- |
| tcp  | 62.6 Gbits/sec | 3.04 Gbits/sec |
| udp  | 15.0 Gbits/sec | 1.0 Gbits/sec |