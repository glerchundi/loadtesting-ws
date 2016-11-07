FROM alpine:3.4
MAINTAINER Gorka Lerchundi Osa <glertxundi@gmail.com>

# benchmarker
ARG version
ADD benchmarker-${version}-linux-amd64 /benchmarker

ENTRYPOINT [ "/benchmarker" ]
