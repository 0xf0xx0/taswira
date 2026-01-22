taswira: tiny image host authed by forgejo

## features

- relies on a forgejo instance for authing users
- handles images up to 256mib
- handles duplicate image uploads
- handles image deletion (also: systemd timer to delete old images)
- uses xxhash for filename

## setup

- MUST be run behind a reverse proxy, it depends on one to serve the images and for X-Forwarded-Host/Host and X-Forwarded-Proto
- `IMG_ROOT` must exist before running, this wont make dirs

### environment variables

```sh
INSTANCE="https://example.com" # Forgejo instance to use for auth, without trailing slash
IMG_ROOT="/path/to/image/dir" # dir to write images to, defaults to <process cwd>/img
SUBPATH="foo/bar/baz" # reverse proxy image subpath, without leading and trailing slash
PORT="6969" # listening port, default 6969
```

### example

TODO

```sh
export SUBPATH="images"
```

### reverse proxy structure

```sh
https://example.com
/
├── images # images served here by reverse proxy
└── up # taswira served here
```

## usage

### uploading

```sh
curl -X POST --data-binary @/path/to/image.jpg -u forgejo_user:forgejo_user_token https://example.com/up # -> '{"message":"ok","url":"https://example.com/images/47af8523b269bd268d90d10818b2f28f.png"}'
```

### deleting

deleting takes a url param `hash`, containing the image hash

```sh
curl -X DELETE -u forgejo_user:forgejo_user_token https://example.com/up?hash=47af8523b269bd268d90d10818b2f28f # -> '{"message":"ok"}'
```

- the token only needs the read:user perm for auth
- accepts png, jpg, and probably webp
- no response until user is authed
- converts all images to png and compresses
- simple `{message: string, url:string?}` object response
- error is `{error: string}`

# known quirks

- any user can delete any image if they know the hash (wontfix, im lazy)

# todo

- make client helper script better (ew bash wehhh)
- temporary uploads (?expiry=<uint>)
- named aliases? (?alias=<string>)
