# gobin

A simple [httpbin](https://httpbin.org) clone written in Go, using only the standard library.

## Run

```sh
go run .
```

The server listens on `:8080` by default. Override with `GOBIN_ADDR`:

```sh
GOBIN_ADDR=:9000 go run .
```

## Endpoints

### Request inspection

#### `GET /ip`

Returns the origin IP of the request.

```json
{"origin": "127.0.0.1"}
```

#### `GET /user-agent`

Returns the request's `User-Agent` header.

```json
{"user-agent": "curl/8.21.0"}
```

#### `GET /headers`

Returns the request headers.

```json
{"headers": {"Accept": "*/*", "User-Agent": "curl/8.21.0"}}
```

#### `GET /bearer`

Requires an `Authorization: Bearer <token>` header.

```json
{"authenticated": true, "token": "abc123"}
```

Without the header, returns `401` with:

```json
{"message": "missing bearer token"}
```

### Echo endpoints

Responses are pretty-printed JSON. Headers are joined with `", "` when sent multiple times (e.g. `"Foo": "1, 2"`) and include `Host`, sorted by name.

#### `GET /get`

Echoes the request metadata.

```json
{
  "args": {},
  "headers": {"Accept": "*/*", "Host": "localhost:8080", "User-Agent": "curl/8.21.0"},
  "origin": "127.0.0.1",
  "url": "http://localhost:8080/get"
}
```

#### `POST /post`, `PUT /put`, `PATCH /patch`, `DELETE /delete`

Echoes the request, including body content. Each path only accepts its matching method.

| Field    | Description                                                       |
| -------- | ----------------------------------------------------------------- |
| `args`   | Query parameters (first value)                                    |
| `data`   | Raw request body                                                  |
| `files`  | Uploaded files from `multipart/form-data` (filename → content)    |
| `form`   | Parsed `application/x-www-form-urlencoded` or multipart form fields |
| `headers`| Request headers (first value)                                     |
| `json`   | Parsed JSON body if `Content-Type` is `application/json` or `*+json`, otherwise `null` |
| `origin` | Client IP                                                         |
| `url`    | Full request URL                                                  |

Example: `POST /post?foo=bar` with form body `a=1&b=2`

```json
{
  "args": {"foo": "bar"},
  "data": "a=1&b=2",
  "files": {},
  "form": {"a": "1", "b": "2"},
  "headers": {"Content-Length": "7", "Content-Type": "application/x-www-form-urlencoded", "Host": "localhost:8080", "User-Agent": "curl/8.21.0"},
  "json": null,
  "origin": "127.0.0.1",
  "url": "http://localhost:8080/post?foo=bar"
}
```

With a JSON body and `Content-Type: application/json`, the parsed value appears in `json`:

```json
{
  "data": "{\"x\":[1,2]}",
  "json": {"x": [1, 2]},
  "form": {},
  "files": {}
}
```

#### `GET /response-headers?k=v`

Sets each query parameter as a response header and returns the parameters as JSON.

```sh
curl -i 'http://localhost:8080/response-headers?X-Test=yes'
```

### Status and errors

#### `GET /status/{code}`

Returns the given HTTP status code (`100`–`599`).

```sh
curl -i http://localhost:8080/status/418
```

### Timing

#### `GET /delay/{seconds}`

Waits the given number of seconds (fractional allowed, max `10`), then responds like `/get`.

### Response generation

#### `GET /bytes/{n}`

Returns `n` random bytes (`max 100000`) as `application/octet-stream`.

#### `GET /stream/{n}`

Streams `n` (`max 1000`) pretty-printed JSON echo objects with an `id` field (`0`–`n-1`), one per 100ms, each followed by a newline.

#### `GET /uuid`

Returns a random UUID (RFC 9562).

```json
{"uuid": "f7882a0b-8751-4ef3-88b1-3bf5710d81bd"}
```

#### `GET /base64/{value}`

Decodes a base64url-encoded value and returns it as plain text.

```sh
curl http://localhost:8080/base64/aGVsbG8=
# hello
```

### Compression

#### `GET /gzip`

Returns an echo response with `Content-Encoding: gzip`.

### Redirects

#### `GET /redirect/{n}`

302-redirects `n` times (`max 10`), then returns an echo response.

### Static content

#### `GET /html` - sample HTML page

#### `GET /json` - sample JSON document

#### `GET /robots.txt` - robots rules
