# SONiC-telemetry - simplified version

## Description
This file gives instructions to build docker image locally on this repository
and run in on the virtual switch without relying on virtual switch generic build infra.

https://github.com/Azure/sonic-buildimage

## Getting Started

### Prerequisites

Use `Makefile` in parent directory to first build the telemetry image. The Makefile doesn't give
instructions to build telemetry binary to build per OS basis. So run the build on the machine
one needs to deploy. I'll modify this section soon once cross compilation build is enabled.

## Building telemetry docker image

```
make container
```

## Building telemetry docker test image

```
make test-container
```

## Running telemetry docker image

The valid working telemetry docker image is also present in `us.gcr.io/s2-infra/s2sonic/docker-telemetry:latest`

```
docker run -d --net=host -v /var/run/redis:/var/run/redis -it docker-telemetry:latest

OR

docker run -d --net=host -v /var/run/redis:/var/run/redis -it us.gcr.io/s2-infra/s2sonic/docker-telemetry:latest
```

## Running telemetry docker test image

The valid working telemetry test docker image is also present in `us.gcr.io/s2-infra/s2sonic/docker-telemetry-test:latest`


Make sure the docker is run on the virtual switch.
Currently it doesn't have support to run remotely

The `testdata` files should be volume mounted. In below
example, the user has placed the testdata directory in
`/home/admin/sonic-files/`

```
docker run --net=host -v /var/run/redis:/var/run/redis -v /usr/bin/:/usr/bin/ -v /var/run/docker.sock:/var/run/docker.sock -v /home/admin/sonic-files/:/var/sonic-files/ docker-telemetry-test:latest

OR
docker run --net=host -v /var/run/redis:/var/run/redis -v /usr/bin/:/usr/bin/ -v /var/run/docker.sock:/var/run/docker.sock -v /home/admin/sonic-files/:/var/sonic-files/ us.gcr.io/s2-infra/s2sonic/docker-telemetry-test:latest
```

