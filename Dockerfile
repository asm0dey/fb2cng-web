# syntax=docker/dockerfile:1

# --- Stage 1: fetch the fbc binary for the TARGET arch (runs on the build host) ---
FROM --platform=$BUILDPLATFORM bellsoft/alpaquita-linux-base:stream-musl AS fbc
ARG FBC_VERSION=v1.4.5
ARG TARGETARCH
RUN apk add --no-cache curl unzip ca-certificates \
 && curl -fsSL -o /tmp/fbc.zip \
      "https://github.com/rupor-github/fb2cng/releases/download/${FBC_VERSION}/fbc-linux-${TARGETARCH}.zip" \
 && unzip -o /tmp/fbc.zip -d /opt \
 && chmod +x /opt/fbc

# --- Stage 2: cross-compile the Go server (runs on the build host) ---
FROM --platform=$BUILDPLATFORM bellsoft/alpaquita-linux-go:1.26.5-musl AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/fb2cng-web .

# --- Stage 3: hardened runtime (per-arch image, COPY only) ---
FROM bellsoft/hardened-base:musl
COPY --from=fbc /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=fbc /opt/fbc /usr/local/bin/fbc
COPY --from=build /out/fb2cng-web /usr/local/bin/fb2cng-web
ENV FBC_BIN=/usr/local/bin/fbc PORT=8080 TMPDIR=/tmp
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/fb2cng-web"]
