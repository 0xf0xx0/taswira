taswira: tiny image host authed by forgejo

## setup
- MUST be run behind a reverse proxy, it depends on one to serve the images and for X-Forwarded-Host/Host and X-Forwarded-Proto
- IMG_ROOT must exist before running, this wont make dirs

### environment variables
```sh
INSTANCE="https://example.com" # Forgejo instance to use for auth, without trailing slash
IMG_ROOT="/path/to/image/dir" # dir to write images to, defaults to <process cwd>/img
SUBPATH="foo/bar/baz" # reverse proxy subpath, without trailing slash
PORT="6969" # listening port, default 6969
```

## example
TODO
```sh
export SUBPATH="images"
```
### reverse proxy structure
```sh
https://example.com
/
├── images # images served here by reverse proxy
└── up # endpoint served here

# usage:
curl -X POST --data-binary @/path/to/image.jpg -u forgejo_user:user_token https://example.com/up # -> '{"message":"ok","url":"https://example.com/images/47af8523b269bd268d90d10818b2f28f.png"}'
```

- no response until authed
- converts all images to png and compresses
- simple `{message: string, url:string?}` object response
- error is `{error: string}`

# known quirks
- any user can delete any image if they know the hash (wontfix, im lazy)

# todo
- temporary uploads (?expiry=<uint>)
- named aliases? (?alias=<string>)
