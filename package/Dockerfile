FROM alpine:3.12 as network-controller
# ebtables v2.0.10-4
# iptables v1.6.2
# The default repository of alpine 3.12 only contains iptables v1.8.4 which includes the ebtables 1.8.4, but we want the version more that 2.0.10-4.
RUN echo -e 'http://dl-cdn.alpinelinux.org/alpine/v3.9/main' > /etc/apk/repositories && apk update && apk add --no-cache ebtables iptables
COPY bin/harvester-network-controller /usr/bin/
CMD ["harvester-network-controller"]

FROM alpine:3.12 as network-helper
COPY bin/harvester-network-helper /usr/bin/
CMD ["harvester-network-helper"]
