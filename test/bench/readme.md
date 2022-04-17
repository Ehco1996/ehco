
# setup env

1. run `iperf3` server by `iperf3 -s`
2. start ss client by `start_ss_client.sh`
3. start ehco and xray(ss) server by `go run cmd/ehco/main.go -c test/bench/ehco_config.json`

# run test

* raw ss `iperf3 -c 127.0.0.1 -p 1090`

```bash
Connecting to host 127.0.0.1, port 1090
[  5] local 127.0.0.1 port 51860 connected to 127.0.0.1 port 1090
[ ID] Interval           Transfer     Bitrate
[  5]   0.00-1.00   sec   605 MBytes  5.08 Gbits/sec
[  5]   1.00-2.00   sec   621 MBytes  5.21 Gbits/sec
[  5]   2.00-3.00   sec   621 MBytes  5.21 Gbits/sec
[  5]   3.00-4.00   sec   622 MBytes  5.22 Gbits/sec
[  5]   4.00-5.00   sec   616 MBytes  5.17 Gbits/sec
[  5]   5.00-6.00   sec   621 MBytes  5.21 Gbits/sec
[  5]   6.00-7.00   sec   618 MBytes  5.18 Gbits/sec
[  5]   7.00-8.00   sec   621 MBytes  5.21 Gbits/sec
[  5]   8.00-9.00   sec   619 MBytes  5.20 Gbits/sec
[  5]   9.00-10.00  sec   613 MBytes  5.14 Gbits/sec
- - - - - - - - - - - - - - - - - - - - - - - - -
[ ID] Interval           Transfer     Bitrate
[  5]   0.00-10.00  sec  6.03 GBytes  5.18 Gbits/sec                  sender
[  5]   0.00-10.00  sec  6.03 GBytes  5.18 Gbits/sec                  receiver
```

* ehco raw `iperf3 -c 127.0.0.1 -p 1234`

```bash
Connecting to host 127.0.0.1, port 1234
[  5] local 127.0.0.1 port 53027 connected to 127.0.0.1 port 1234
[ ID] Interval           Transfer     Bitrate
[  5]   0.00-1.00   sec   616 MBytes  5.17 Gbits/sec
[  5]   1.00-2.00   sec   603 MBytes  5.06 Gbits/sec
[  5]   2.00-3.00   sec   612 MBytes  5.14 Gbits/sec
[  5]   3.00-4.00   sec   616 MBytes  5.17 Gbits/sec
[  5]   4.00-5.00   sec   616 MBytes  5.17 Gbits/sec
[  5]   5.00-6.00   sec   614 MBytes  5.15 Gbits/sec
[  5]   6.00-7.00   sec   615 MBytes  5.15 Gbits/sec
[  5]   7.00-8.00   sec   616 MBytes  5.17 Gbits/sec
[  5]   8.00-9.00   sec   609 MBytes  5.11 Gbits/sec
[  5]   9.00-10.00  sec   618 MBytes  5.18 Gbits/sec
- - - - - - - - - - - - - - - - - - - - - - - - -
[ ID] Interval           Transfer     Bitrate
[  5]   0.00-10.00  sec  5.99 GBytes  5.15 Gbits/sec                  sender
[  5]   0.00-10.00  sec  5.98 GBytes  5.14 Gbits/sec                  receiver

iperf Done.
```

* ehco over ws `iperf3 -c 127.0.0.1 -p 1235`

```bash
Connecting to host 127.0.0.1, port 1235
[  5] local 127.0.0.1 port 53778 connected to 127.0.0.1 port 1235
[ ID] Interval           Transfer     Bitrate
[  5]   0.00-1.00   sec   610 MBytes  5.11 Gbits/sec
[  5]   1.00-2.00   sec   588 MBytes  4.94 Gbits/sec
[  5]   2.00-3.00   sec   594 MBytes  4.98 Gbits/sec
[  5]   3.00-4.00   sec   594 MBytes  4.98 Gbits/sec
[  5]   4.00-5.00   sec   593 MBytes  4.97 Gbits/sec
[  5]   5.00-6.00   sec   588 MBytes  4.94 Gbits/sec
[  5]   6.00-7.00   sec   593 MBytes  4.97 Gbits/sec
[  5]   7.00-8.00   sec   593 MBytes  4.98 Gbits/sec
[  5]   8.00-9.00   sec   593 MBytes  4.98 Gbits/sec
[  5]   9.00-10.00  sec   595 MBytes  4.99 Gbits/sec
- - - - - - - - - - - - - - - - - - - - - - - - -
[ ID] Interval           Transfer     Bitrate
[  5]   0.00-10.00  sec  5.80 GBytes  4.98 Gbits/sec                  sender
[  5]   0.00-10.00  sec  5.78 GBytes  4.96 Gbits/sec                  receiver

iperf Done.
```

* ehco over wss `iperf3 -c 127.0.0.1 -p 1236`

```bash
Connecting to host 127.0.0.1, port 1236
[  5] local 127.0.0.1 port 53866 connected to 127.0.0.1 port 1236
[ ID] Interval           Transfer     Bitrate
[  5]   0.00-1.00   sec   573 MBytes  4.81 Gbits/sec
[  5]   1.00-2.00   sec   569 MBytes  4.78 Gbits/sec
[  5]   2.00-3.00   sec   575 MBytes  4.82 Gbits/sec
[  5]   3.00-4.00   sec   585 MBytes  4.91 Gbits/sec
[  5]   4.00-5.00   sec   590 MBytes  4.95 Gbits/sec
[  5]   5.00-6.00   sec   586 MBytes  4.92 Gbits/sec
[  5]   6.00-7.00   sec   587 MBytes  4.93 Gbits/sec
[  5]   7.00-8.00   sec   590 MBytes  4.95 Gbits/sec
[  5]   8.00-9.00   sec   587 MBytes  4.93 Gbits/sec
[  5]   9.00-10.00  sec   580 MBytes  4.87 Gbits/sec
- - - - - - - - - - - - - - - - - - - - - - - - -
[ ID] Interval           Transfer     Bitrate
[  5]   0.00-10.00  sec  5.69 GBytes  4.88 Gbits/sec                  sender
[  5]   0.00-10.00  sec  5.68 GBytes  4.88 Gbits/sec                  receiver

iperf Done.
```

* ehco over mwss `iperf3 -c 127.0.0.1 -p 1237`

```bash
Connecting to host 127.0.0.1, port 1237
[  5] local 127.0.0.1 port 54878 connected to 127.0.0.1 port 1237
[ ID] Interval           Transfer     Bitrate
[  5]   0.00-1.00   sec   555 MBytes  4.65 Gbits/sec
[  5]   1.00-2.00   sec   542 MBytes  4.55 Gbits/sec
[  5]   2.00-3.00   sec   555 MBytes  4.66 Gbits/sec
[  5]   3.00-4.00   sec   549 MBytes  4.61 Gbits/sec
[  5]   4.00-5.00   sec   485 MBytes  4.07 Gbits/sec
[  5]   5.00-6.00   sec   514 MBytes  4.31 Gbits/sec
[  5]   6.00-7.00   sec   555 MBytes  4.66 Gbits/sec
[  5]   7.00-8.00   sec   555 MBytes  4.65 Gbits/sec
[  5]   8.00-9.00   sec   541 MBytes  4.54 Gbits/sec
[  5]   9.00-10.00  sec   533 MBytes  4.47 Gbits/sec
- - - - - - - - - - - - - - - - - - - - - - - - -
[ ID] Interval           Transfer     Bitrate
[  5]   0.00-10.00  sec  5.26 GBytes  4.52 Gbits/sec                  sender
[  5]   0.00-10.02  sec  5.25 GBytes  4.50 Gbits/sec                  receiver

iperf Done.
```
