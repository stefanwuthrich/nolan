# nolan cluster example

This will start a local three node cluster.

## Build

```bash
$ go get github.com/travisjeffery/jocko/...
$ cd $GOPATH/src/github.com/travisjeffery/jocko/cmd/jocko
$ go build
```

## Start a single node

```bash
$ nolan broker \
          --data-dir="/tmp/nolan0" \
          --broker-addr=127.0.0.1:9092 \
          --raft-addr=127.0.0.1:9093 \
          --serf-addr=127.0.0.1:9094 \
          --bootstrap \
          --bootstrap-expect=1 \
          --id=1
```

## Start the nodes

```bash
$ nolan broker \
          --data-dir="/tmp/nolan0" \
          --broker-addr=127.0.0.1:9001 \
          --raft-addr=127.0.0.1:9002 \
          --serf-addr=127.0.0.1:9003 \
          --bootstrap \
          --bootstrap-expect=2 \
          --id=1

$ nolan broker \
          --data-dir="/tmp/nolan1" \
          --broker-addr=127.0.0.1:9101 \
          --raft-addr=127.0.0.1:9102 \
          --serf-addr=127.0.0.1:9103 \
          --join=127.0.0.1:9003 \
          --join-wan=127.0.0.1:9003 \
          --bootstrap-expect=2 \
          --id=2

$ nolan broker \
          --data-dir="/tmp/nolan2" \
          --broker-addr=127.0.0.1:920 \
          --raft-addr=127.0.0.1:9202 \
          --serf-addr=127.0.0.1:9203 \
          --join=127.0.0.1:9003 \
          --bootstrap-expect=3 \
          --id=3
```

## docker-compose cluster

To start a [docker compose](https://docs.docker.com/compose/) cluster use the provided `docker-compose.yml`.
