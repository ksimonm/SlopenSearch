FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /slopensearch .

FROM alpine:3.22
COPY --from=build /slopensearch /usr/local/bin/slopensearch
# state lives in $SLOP_DATA_DIR (mount a volume there); SLOP_IN_MEMORY=true disables it
RUN mkdir -p /data && chown nobody:nobody /data
ENV HOST=0.0.0.0 PORT=9200 SLOP_DATA_DIR=/data
EXPOSE 9200
USER nobody
ENTRYPOINT ["slopensearch"]
