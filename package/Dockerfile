FROM alpine:3.9 as network-controller
RUN apk add ebtables iptables
COPY bin/harvester-network-controller /usr/bin/
CMD ["harvester-network-controller"]

FROM alpine:3.9 as network-helper
COPY bin/harvester-network-helper /usr/bin/
CMD ["harvester-network-helper"]
