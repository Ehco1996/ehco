# Ehco

ehco is a network relay tool and a typo :)

[![Go Report Card](https://goreportcard.com/badge/github.com/Ehco1996/ehco)](https://goreportcard.com/report/github.com/Ehco1996/ehco)
[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/Ehco1996/ehco)
[![Docker Pulls](https://img.shields.io/docker/pulls/ehco1996/ehco)](https://hub.docker.com/r/ehco1996/ehco)

## Quick Start

let's see some examples

> relay all tcp traffic from `0.0.0.0:1234` to `0.0.0.0:5201`

`ehco -l 0.0.0.0:1234 -r 0.0.0.0:5201`

> also relay udp traffic  to `0.0.0.0:5201`

`ehco -l 0.0.0.0:1234 -r 0.0.0.0:5201 -ur 0.0.0.0:5201`

## Advance Usage

TBD, for now, you can see more examples in [ReadmeCN](README.md)
