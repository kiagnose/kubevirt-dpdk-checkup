FROM quay.io/centos/centos:stream9

ARG TREX_VERSION=2.87
ENV TREX_VERSION ${TREX_VERSION}

# install requirements
RUN dnf update -y && \
    dnf install -y --nodocs \
      wget \
      procps \
      python3 \
      python3-pip \
      pciutils \
      gettext \
      hostname \
      iproute \
      ethtool \
      nmap iputils \
      perf \
      numactl \
      sysstat \
      rdma-core \
      libibverbs \
      net-tools && \
    dnf clean all

# install trex server
WORKDIR /opt/
RUN wget --no-check-certificate https://trex-tgn.cisco.com/trex/release/v${TREX_VERSION}.tar.gz && \
    tar -xzf v${TREX_VERSION}.tar.gz && \
    mv v${TREX_VERSION} trex && \
    rm v${TREX_VERSION}.tar.gz

WORKDIR /opt/trex
