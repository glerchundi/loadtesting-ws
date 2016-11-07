Building `loadtesting-ws`:

Define building configuration:
```
export VER=v0.0.1
export VER_TRIM=$(echo -e "${VER}" | sed -e 's/^[[v]]*//')
export APP="listener benchmarker"
```

Start building:
```
docker build --build-arg version=${VER_TRIM} -f Dockerfile.build -t loadtesting-ws:build-${VER} .
```

Generate docker images for concrete apps:
```
for app in ${APP[*]}; do \
  docker run --rm loadtesting-ws:build-${VER} \
       cat /go/src/github.com/glerchundi/loadtesting-ws/bin/${app}-${VER_TRIM}-linux-amd64 \
       > cmd/${app}/container/${app}-${VER_TRIM}-linux-amd64; \
  chmod 0755 cmd/${app}/container/${app}-${VER_TRIM}-linux-amd64; \
  docker build --build-arg version=${VER_TRIM} -f cmd/${app}/container/Dockerfile -t quay.io/glerchundi/loadtesting-ws-${app}:${VER} cmd/${app}/container/; \
done
```

Publish built apps:
```
for app in ${APP[*]}; do \
  docker push quay.io/glerchundi/loadtesting-ws-${app}:${VER}; \
done
```
