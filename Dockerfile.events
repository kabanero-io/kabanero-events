# Copyright 2019 IBM Corporation
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Production build dockerfile
# Stage 1: build using golang image
FROM golang as builder

WORKDIR $GOPATH/src/github.com/kabanero-io/kabanero-events

# install dep tool
RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

# Copy dependecies over
COPY  Gopkg.* ./

# load vendor dependent packages
RUN dep ensure -v -vendor-only

# Copy source over
COPY *.go ./

# Linter
RUN go get -u golang.org/x/lint/golint; golint -set_exit_status

# Run unit test
# COPY test_data ./test_data/
# RUN go test -v

# Build executable
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' github.com/kabanero-io/kabanero-events/cmd/kabanero-events/...

# Stage 2: Build official image based on UBI
FROM registry.access.redhat.com/ubi7-minimal:7.6-123

LABEL name="Kabanero Events" \
      version="0.1.0" \
      release="0.1.0" \
      vendor="kabanero" \
      summary="Kabanero Events" \
      description="Image for Kabanero event processing"

RUN microdnf -y install shadow-utils \
    && microdnf clean all \
    && mkdir /licenses \
    && useradd -u 1001 -r -g 0 -s /usr/sbin/nologin default \
    && microdnf -y remove shadow-utils \
    && microdnf clean all \
    && mkdir /app \
    && chown 1001:0 /app \
    && chmod g+rwx /app

COPY --from=builder --chown=1001:0 /go/src/github.com/kabanero-io/kabanero-events/kabanero-events /app
COPY --chown=1001:0 licenses/ /licenses/
# COPY --chown=1001:0 testcntlr.sh /bin/testcntlr.sh
USER 1001
WORKDIR /app
EXPOSE 9443

# run with log level 2
# Note liveness/readiness probe depends on './kabanero-events'
CMD ./kabanero-events -v 5
