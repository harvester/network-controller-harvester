FROM alpine:3.9
RUN apk add ebtables iptables
COPY bin/harvester-network-controller /usr/bin/
CMD ["harvester-network-controller"]
