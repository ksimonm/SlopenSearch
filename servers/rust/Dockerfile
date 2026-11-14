FROM rust:1-bookworm AS build
WORKDIR /src
COPY Cargo.toml Cargo.lock ./
COPY src ./src
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=/src/target \
    cargo build --release && cp target/release/slopensearch /slopensearch

FROM debian:bookworm-slim
COPY --from=build /slopensearch /usr/local/bin/slopensearch
# durable state; a fresh named volume mounted here inherits the nobody ownership
RUN mkdir /data && chown nobody /data
ENV HOST=0.0.0.0 PORT=9200 SLOP_DATA_DIR=/data
EXPOSE 9200
USER nobody
ENTRYPOINT ["slopensearch"]
