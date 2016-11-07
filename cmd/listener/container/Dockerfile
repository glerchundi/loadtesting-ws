FROM alpine:3.4
MAINTAINER Gorka Lerchundi Osa <glertxundi@gmail.com>

# listener
ARG version
ADD listener-${version}-linux-amd64 /listener

ENTRYPOINT [ "/listener" ]
