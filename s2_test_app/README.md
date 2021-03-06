# SONiC-test application

## Description
Using this application, one can virtually impose the counter behavior present in the virtual switch. The virtual switch is based of below hardware model

```
SONiC Software Version: SONiC.HEAD.230-c9483796
Distribution: Debian 9.11
Kernel: 4.9.0-9-2-amd64
Build commit: c9483796
Build date: Tue Jan 28 10:34:47 UTC 2020
Built by: johnar@jenkins-worker-12

Platform: x86_64-kvm_x86_64-r0
HwSKU: Force10-S6000
ASIC: vs
Serial Number: 000000
Uptime: 14:32:22 up  4:56,  1 user,  load average: 0.03, 0.05, 0.04
```

## How to use the application
Release image is available in `us.gcr.io/s2-infra/s2sonic/docker-telemetry-test:latest`

To use this app, use below docker command on the virtual switch. This application will run along with the telemetry application.
Note: The policy file should be mapped to docker for use. In below example, the policy file should be in /sonic/input/, with filename as `policy.toml`

```
docker run --net=host -v /var/run/redis:/var/run/redis -v /usr/bin/:/usr/bin/ -v /var/run/docker.sock:/var/run/docker.sock -v /home/admin/policy-files/:/sonic/input/ docker-telemetry-test:latest

OR

docker run --net=host -v /var/run/redis:/var/run/redis -v /usr/bin/:/usr/bin/ -v /var/run/docker.sock:/var/run/docker.sock -v /home/admin/policy-files/:/sonic/input/ us.gcr.io/s2-infra/s2sonic/docker-telemetry-test:latest
```

### Sample policy input file

```
# This is a TOML document. Boom.

title = "BST statistics"

[app]
name = "sonic vswitch counters"

[counters]
  # You can indent as you please. Tabs or spaces. TOML don't care
  [counters.1]
  description = "fake counter that always is set to 10"
  counter_path = "/Counters/Ethernet9/SAI_PORT_STAT_PFC_7_RX_PKTS"
  counter_type = "fixed"
  counter_value = 10

  [counters.2]
  description = "fake counter that generates a random value very 10s"
  counter_path = "/Counters/Ethernet9/Queues/SAI_QUEUE_STAT_CURR_OCCUPANCY_BYTES"
  counter_type = "random"
  interval_sec = 10

  [counters.3]
  description = "fake counter that starts at 10, increments in steps of 100 every 11 secs"
  counter_path = "/Counters/Ethernet9/SAI_PORT_STAT_ETHER_STATS_PKTS"
  counter_type = "incrementing"
  start_count  = 10
  step_count   = 100
  interval_sec = 11
```
## Building test application
TBD

## Below are the supported queue based counters
```
    SAI_QUEUE_STAT_BYTES
    SAI_QUEUE_STAT_DROPPED_BYTES
    SAI_QUEUE_STAT_DROPPED_PACKETS
    SAI_QUEUE_STAT_PACKETS
    SAI_QUEUE_STAT_SHARED_WATERMARK_BYTES
    SAI_QUEUE_STAT_SHARED_CURR_OCCUPANCY_BYTES
    SAI_QUEUE_STAT_CURR_OCCUPANCY_BYTES
    SAI_QUEUE_STAT_WATERMARK_BYTES
```

## Below are the supported port based counters
```
        SAI_PORT_STAT_ETHER_IN_PKTS_128_TO_255_OCTETS
        SAI_PORT_STAT_ETHER_RX_OVERSIZE_PKTS
        SAI_PORT_STAT_ETHER_STATS_TX_NO_ERRORS
        SAI_PORT_STAT_ETHER_TX_OVERSIZE_PKTS
        SAI_PORT_STAT_IF_IN_BROADCAST_PKTS
        SAI_PORT_STAT_IF_IN_DISCARDS
        SAI_PORT_STAT_IF_IN_ERRORS
        SAI_PORT_STAT_IF_IN_MULTICAST_PKTS
        SAI_PORT_STAT_IF_IN_NON_UCAST_PKTS
        SAI_PORT_STAT_IF_IN_OCTETS
        SAI_PORT_STAT_IF_IN_UCAST_PKTS
        SAI_PORT_STAT_IF_IN_UNKNOWN_PROTOS
        SAI_PORT_STAT_IF_OUT_BROADCAST_PKTS
        SAI_PORT_STAT_IF_OUT_DISCARDS
        SAI_PORT_STAT_IF_OUT_ERRORS
        SAI_PORT_STAT_IF_OUT_MULTICAST_PKTS
        SAI_PORT_STAT_IF_OUT_NON_UCAST_PKTS
        SAI_PORT_STAT_IF_OUT_OCTETS
        SAI_PORT_STAT_IF_OUT_QLEN
        SAI_PORT_STAT_IF_OUT_UCAST_PKTS
        SAI_PORT_STAT_IP_IN_UCAST_PKTS
        SAI_PORT_STAT_PAUSE_RX_PKTS
        SAI_PORT_STAT_PAUSE_TX_PKTS
        SAI_PORT_STAT_PFC_0_RX_PKTS
        SAI_PORT_STAT_PFC_0_TX_PKTS
        SAI_PORT_STAT_PFC_1_RX_PKTS
        SAI_PORT_STAT_PFC_1_TX_PKTS
        SAI_PORT_STAT_PFC_2_RX_PKTS
        SAI_PORT_STAT_PFC_2_TX_PKTS
        SAI_PORT_STAT_PFC_3_RX_PKTS
        SAI_PORT_STAT_PFC_3_TX_PKTS
        SAI_PORT_STAT_PFC_4_RX_PKTS
        SAI_PORT_STAT_PFC_4_TX_PKTS
        SAI_PORT_STAT_PFC_5_RX_PKTS
        SAI_PORT_STAT_PFC_5_TX_PKTS
        SAI_PORT_STAT_PFC_6_RX_PKTS
        SAI_PORT_STAT_PFC_6_TX_PKTS
        SAI_PORT_STAT_PFC_7_RX_PKTS
        SAI_PORT_STAT_PFC_7_TX_PKTS
```

## Status
As long it has one counter to watch, the application continues to run. Currently its stateless and doesn't use DB to store the state. For more details, please refer to below link

https://docs.google.com/document/d/1yBBupspBJYgXiUzIGCDIxp6WPn0d12lq8Pw4p5IG9j8/edit#heading=h.yglgzm30a4yu

