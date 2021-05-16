# Reproducing SNAT insertion failures

This small Go program aims at reproducing the condition for a race when inserting records in the conntrack table.
More explanations about the underlying issue can be found in [this blog post](https://tech.xing.com/a-reason-for-unexplained-connection-timeouts-on-kubernetes-docker-abd041cf7e02).

## Introduction

The Linux Kernel has a known race condition when doing source network address translation (SNAT) that can lead to SYN packets being dropped. SNAT is performed by default on outgoing connections with Docker and Flannel using iptables masquerading rules.
The race can happen when multiple containers try to establish new connections to the same external address concurrently.
In some cases, two connections can be allocated the same port for the translation which ultimately results in one or more packets being dropped and at least one second connection delay.

This problem can be reproduced with a default Docker installation and this README try to explains how.

## Reproducing the issue

The easiest way to reproduce the issue requires two servers with Docker installed and root access:
* The *destination server* runs a Docker container that listens for incoming tcp connections. It's the endpoint for the connections we're trying to break. It should be fast enough to accept and immediately close the connection but doesn't require to be multi-core.
* The *source server* is where we try to reproduce the issue. It runs 3 containers:
   1. A container to observe the insertion failure in `conntrack` and manipule `iptables`.
   2. Two containers that try to connect to the destination server and race.

The likelihood for observing the issue increases with the number of cores on the *source server*.

### Prepare the destination server
Start a container that listens for tcp connections:
```bash
$ sudo docker run -p8080:8080 -ti maxlaverse/snat-race-conn-test server
```

If you try to connect from another server, the connection should work and immediately be closed:
```bash
$ nc -vv <ip-of-dst-server> 8080
Connection to <ip-of-dst-server> 8080 port [tcp/http-alt] succeeded!
```

### Prepare the util container on the source server

Start an Alpine container on the *source server*, install `conntrack` and `iptables`:
```bash
$ sudo docker run --privileged --net=host -ti alpine:latest

$ apk add conntrack-tools iptables
fetch https://dl-cdn.alpinelinux.org/alpine/v3.13/main/x86_64/APKINDEX.tar.gz
fetch https://dl-cdn.alpinelinux.org/alpine/v3.13/community/x86_64/APKINDEX.tar.gz
(1/9) Installing libmnl (1.0.4-r1)
(2/9) Installing libnfnetlink (1.0.1-r2)
(3/9) Installing libnetfilter_conntrack (1.0.8-r0)
(4/9) Installing libnetfilter_cthelper (1.0.0-r1)
(5/9) Installing libnetfilter_cttimeout (1.0.0-r1)
(6/9) Installing libnetfilter_queue (1.0.5-r0)
(7/9) Installing conntrack-tools (1.4.6-r0)
(8/9) Installing libnftnl-libs (1.1.8-r0)
(9/9) Installing iptables (1.8.6-r0)
Executing busybox-1.32.1-r6.trigger
OK: 9 MiB in 23 packages
```

This containers requires access to the host network interface and has to be privileged in order to interact with it. 
From this container, have a first look at the conntrack statistics:
```
$ conntrack -S
cpu=0   	found=0 invalid=0 ignore=2 insert=0 insert_failed=0 drop=0 early_drop=0 error=0 search_restart=0
cpu=1   	found=0 invalid=0 ignore=0 insert=0 insert_failed=0 drop=0 early_drop=0 error=0 search_restart=0
cpu=2   	found=0 invalid=0 ignore=0 insert=0 insert_failed=0 drop=0 early_drop=0 error=0 search_restart=0
cpu=3   	found=0 invalid=0 ignore=0 insert=0 insert_failed=0 drop=0 early_drop=0 error=0 search_restart=0
cpu=4   	found=0 invalid=0 ignore=0 insert=0 insert_failed=0 drop=0 early_drop=0 error=0 search_restart=0
cpu=5   	found=0 invalid=0 ignore=0 insert=0 insert_failed=0 drop=0 early_drop=0 error=0 search_restart=0
cpu=6   	found=0 invalid=0 ignore=0 insert=0 insert_failed=0 drop=0 early_drop=0 error=0 search_restart=0
cpu=7   	found=0 invalid=0 ignore=0 insert=0 insert_failed=0 drop=0 early_drop=0 error=0 search_restart=0
cpu=8   	found=0 invalid=0 ignore=0 insert=0 insert_failed=0 drop=0 early_drop=0 error=0 search_restart=0
cpu=9   	found=0 invalid=0 ignore=6 insert=0 insert_failed=0 drop=0 early_drop=0 error=0 search_restart=0
cpu=10   	found=0 invalid=0 ignore=6 insert=0 insert_failed=0 drop=0 early_drop=0 error=0 search_restart=0
[...] 
```

This command was executed after a reboot. Therefore all `insert_failed` counters are at 0.

From this container, you can also check that the Docker masquerading iptables rule is present:
```
$ iptables-save
[...]
-A POSTROUTING -s 172.17.0.0/16 ! -o docker0 -j MASQUERADE
[...]
```

### Run the test containers on the source server
Start two containers that continuously establish connections to the destination server:
```bash
$ sudo docker run -ti maxlaverse/snat-race-conn-test client --remote-addr <ip-of-dst-server>:8080

$ sudo docker run -ti maxlaverse/snat-race-conn-test client --remote-addr <ip-of-dst-server>:8080
```

The test program periodically prints a summary of the response time and the number of errors:
```
2021-05-16T15:42:32.108 Summary of the last 5 seconds:
             Max response time:   3.9ms
                 99 percentile:   3.8ms
                 95 percentile:   3.2ms
                        median:   2.6ms
                  Request Rate:   156req/s

Requests since start (error/total):     0/784
```

After a couple of seconds you should already see errors being logged in one or both containers:
```
2021-05-16T15:42:36.819 error after 500ms, err: "dial tcp :0->10.18.10.137:8080: i/o timeout"
2021-05-16T15:42:37.108 Summary of the last 5 seconds:
             Max response time: 500.1ms
                 99 percentile:   3.7ms
                 95 percentile:   3.2ms
                        median:   2.5ms
                  Request Rate:   159req/s

Requests since start (error/total):     1/1580
```

After a couple of minutes, press Ctrl+C to stop the two test containers and note down the amount of errors:
```bash
Container 1: Requests since start (error/total):  1439/49571
Container 2: Requests since start (error/total):   829/51927

# Approximately 2.05% of the connection failed
```

Have another look at the conntrack statistics from the util container. You can see that some `insert_failed` counters increased:
```
```bash
$ conntrack -S
cpu=0   	found=4601 invalid=2157 ignore=0 insert=0 insert_failed=74 drop=74 early_drop=0 error=0 search_restart=100
cpu=1   	found=7545 invalid=3952 ignore=0 insert=0 insert_failed=175 drop=175 early_drop=0 error=0 search_restart=159
cpu=2   	found=3810 invalid=1871 ignore=2 insert=0 insert_failed=56 drop=56 early_drop=0 error=0 search_restart=72
cpu=3   	found=8138 invalid=4610 ignore=0 insert=0 insert_failed=166 drop=166 early_drop=0 error=0 search_restart=181
cpu=4   	found=4139 invalid=2725 ignore=0 insert=0 insert_failed=82 drop=82 early_drop=0 error=0 search_restart=129
cpu=5   	found=6283 invalid=4059 ignore=6 insert=0 insert_failed=129 drop=129 early_drop=0 error=0 search_restart=220
cpu=6   	found=9915 invalid=5704 ignore=0 insert=0 insert_failed=196 drop=196 early_drop=0 error=0 search_restart=251
cpu=7   	found=9709 invalid=4931 ignore=0 insert=0 insert_failed=215 drop=215 early_drop=0 error=0 search_restart=235
cpu=8   	found=7441 invalid=4626 ignore=0 insert=0 insert_failed=162 drop=162 early_drop=0 error=0 search_restart=246
cpu=9   	found=12412 invalid=6959 ignore=0 insert=0 insert_failed=249 drop=249 early_drop=0 error=0 search_restart=306
cpu=10  	found=11154 invalid=6175 ignore=0 insert=0 insert_failed=231 drop=231 early_drop=0 error=0 search_restart=272
[...]
```

### Trying out a mitigation

In the util container, save the iptables rules:
```bash
$ iptables-save > dump
```

Append `--random-fully` to the masquerading rule:
```diff
- -A POSTROUTING -s 172.17.0.0/16 ! -o docker0 -j MASQUERADE
+ -A POSTROUTING -s 172.17.0.0/16 ! -o docker0 -j MASQUERADE --random-fully
```

Reload the rules:
```bash
$ iptables-restore < dump
```

Now restart the two test containers and stop them after the same amount of requests than the first run:
```bash
Container 1: Requests since start (error/total):    45/50210
Container 2: Requests since start (error/total):    57/50160

# Approximately 0.10% of the connection failed with --random-fully
# This is a 95% error decrease in this particular case, and can vary a lot from one setup from the other.
```

## Kubernetes

You can run the same test on Kubernetes. Your test Pods need to be collocated on the same Node in order to observe insertion failures.

## Building the project

The test program is written using Go module. You need at least Go 1.16 to compile it:
```bash
$ go build
```

You can build you own Docker image with:
```bash
$ sudo docker build -t testimage .
```
