FROM golang:latest

RUN apt-get update && apt-get install -y \
bash

RUN mkdir -p /sonic/telemetry/bin

WORKDIR  /sonic/telemetry

COPY telemetry/telemetry bin/

ENV PATH $PATH:/sonic/telemetry/bin

WORKDIR /sonic/telemetry/bin

CMD telemetry --port 8080 -insecure --allow_no_client_auth --logtostderr
