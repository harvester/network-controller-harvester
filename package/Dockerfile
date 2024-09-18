# syntax=docker/dockerfile:1.7.0

FROM registry.suse.com/bci/bci-base:15.6
RUN zypper -n rm container-suseconnect && \
    zypper install -y iptables=1.8.7 && \
    zypper -n clean -a && rm -rf /tmp/* /var/tmp/* /usr/share/doc/packages/*

ARG TARGETPLATFORM

RUN if [ "$TARGETPLATFORM" != "linux/amd64" ] && [ "$TARGETPLATFORM" != "linux/arm64" ]; then \
    echo "Error: Unsupported TARGETPLATFORM: $TARGETPLATFORM" && \
    exit 1; \
    fi

ENV ARCH=${TARGETPLATFORM#linux/}

COPY bin/harvester-network-controller-${ARCH} /usr/bin/harvester-network-controller
CMD ["harvester-network-controller"]
