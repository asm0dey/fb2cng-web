# fb2cng-web

Web interface for [fb2cng](https://github.com/rupor-github/fb2cng): drop `fb2` / `fb2.zip`
files, get EPUB (or kepub/kfx/azw8/pdf) back automatically.

## Run

    docker build -t fb2cng-web .
    docker run --rm -p 8080:8080 fb2cng-web

> The image is built on BellSoft Alpaquita Linux (musl); the production runtime stage uses
> the hardened Alpaquita base (`bellsoft/hardened-base:musl`) — minimal, non-root (UID 65532),
> no shell or package manager. The app writes conversion temp files to `/tmp`; the hardened
> base ships a writable `/tmp`, so this works out of the box. If you run with a read-only root
> filesystem, mount a writable `/tmp` (e.g. `--tmpfs /tmp:rw,mode=1777`, as the compose example shows).

Open http://localhost:8080. Defaults work out of the box; expand **Settings** to change
format, ToC, images, footnotes, or paste/upload a full fbc YAML config.

## Configuration (env)

| Var | Default | Meaning |
|-----|---------|---------|
| `PORT` | `8080` | listen port |
| `FBC_BIN` | `fbc` | path to the fbc binary |
| `MAX_CONCURRENT` | `3` | max simultaneous conversions |
| `AUTH_FORWARD_AUTH` | `false` | trust reverse-proxy `Remote-*` headers |
| `TRUSTED_PROXIES` | (empty) | comma-separated source IPs allowed to set `Remote-*` |

> In the Docker image, `FBC_BIN` is preset to `/usr/local/bin/fbc`.

## Optional authentication (Authelia forward-auth)

The app has no built-in login. To require auth, run it behind a reverse proxy that
delegates to **Authelia**, with `AUTH_FORWARD_AUTH=true`.

> **Security:** when auth is on, never expose the app port directly. Publish only the
> proxy and keep the app on an internal network. Optionally set `TRUSTED_PROXIES` so the
> app ignores `Remote-*` headers from any other source.

Authelia forward-auth endpoint: `/api/authz/forward-auth`. Copy headers
`Remote-User`, `Remote-Groups`, `Remote-Email`, `Remote-Name` to the app.

### Caddy (`Caddyfile`)

```caddyfile
fb2.example.com {
    forward_auth authelia:9091 {
        uri /api/authz/forward-auth
        copy_headers Remote-User Remote-Groups Remote-Email Remote-Name
    }
    reverse_proxy fb2cng-web:8080
}
```

### Traefik (dynamic config)

```yaml
http:
  middlewares:
    authelia:
      forwardAuth:
        address: "http://authelia:9091/api/authz/forward-auth"
        authResponseHeaders:
          - "Remote-User"
          - "Remote-Groups"
          - "Remote-Email"
          - "Remote-Name"
  routers:
    fb2:
      rule: "Host(`fb2.example.com`)"
      middlewares: ["authelia"]
      service: fb2cng-web
  services:
    fb2cng-web:
      loadBalancer:
        servers:
          - url: "http://fb2cng-web:8080"
```

### Nginx Proxy Manager (Proxy Host → Advanced tab)

```nginx
location /authelia {
    # nginx uses Authelia's auth-request endpoint (not the forward-auth one used by Caddy/Traefik)
    internal;
    proxy_pass http://authelia:9091/api/authz/auth-request;
    proxy_pass_request_body off;
    proxy_set_header Content-Length "";
    proxy_set_header X-Original-URL $scheme://$http_host$request_uri;
    proxy_set_header X-Original-Method $request_method;
    proxy_set_header X-Forwarded-For $remote_addr;
}
location / {
    auth_request /authelia;
    auth_request_set $user  $upstream_http_remote_user;
    auth_request_set $groups $upstream_http_remote_groups;
    auth_request_set $name  $upstream_http_remote_name;
    auth_request_set $email $upstream_http_remote_email;
    proxy_set_header Remote-User   $user;
    proxy_set_header Remote-Groups $groups;
    proxy_set_header Remote-Name   $name;
    proxy_set_header Remote-Email  $email;
    error_page 401 =302 https://auth.example.com/?rd=$scheme://$http_host$request_uri;
    proxy_pass http://fb2cng-web:8080;
}
```
