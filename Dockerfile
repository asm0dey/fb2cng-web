# syntax=docker/dockerfile:1

# --- Stage 1: fetch the fbc binary ---
FROM bellsoft/alpaquita-linux-base:stream-musl AS fbc
ARG FBC_VERSION=v1.4.5
ARG FBC_ASSET=fbc-linux-amd64.zip
RUN apk add --no-cache curl unzip ca-certificates \
 && curl -fsSL -o /tmp/fbc.zip \
      "https://github.com/rupor-github/fb2cng/releases/download/${FBC_VERSION}/${FBC_ASSET}" \
 && unzip -o /tmp/fbc.zip -d /opt \
 && chmod +x /opt/fbc

# --- Stage 2: build the Go server ---
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/fb2cng-web .

# --- Stage 3: runtime ---
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*
COPY --from=fbc /opt/fbc /usr/local/bin/fbc
COPY --from=build /out/fb2cng-web /usr/local/bin/fb2cng-web
ENV FBC_BIN=/usr/local/bin/fbc PORT=8080
EXPOSE 8080
USER nobody
ENTRYPOINT ["/usr/local/bin/fb2cng-web"]
