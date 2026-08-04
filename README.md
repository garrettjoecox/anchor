![anchor](https://github.com/garrettjoecox/OOT/assets/7316699/a8feac51-47b6-4e4c-b940-2f49fc0bc764)

## What is this?

Anchor is a client/server service for providing multiplayer functions in Harbor Masters 64 ports. It's primary functions are loading save state from a remote player when you join their session, and sending flag sets/item gives across all players in a room/team. 

This implementation of a client/server model is very generic, allowing for multiple games to use it's functions at once as the client software is responsible for handling all of the game state.

## How to use this?

> [!NOTE]
> For your typical user, you don't actually need this, as we have a public server hosted for general use at `anchor.hm64.org:43383` which is the default on clients. Self hosting may be a better option for you if you are not based in the US however, as latency may be an issue.

### Precompiled Binaries

Latest build from `main`, published as assets on the rolling [`nightly`](https://github.com/garrettjoecox/anchor/releases/tag/nightly) pre-release:

- [macOS (arm64)](https://github.com/garrettjoecox/anchor/releases/download/nightly/anchor-macOS-arm64.tar.gz)
- [Linux (x86_64)](https://github.com/garrettjoecox/anchor/releases/download/nightly/anchor-linux-x64.tar.gz)
- [Windows (x86_64)](https://github.com/garrettjoecox/anchor/releases/download/nightly/anchor-windows-x64.exe)

The macOS and Linux downloads are tarballs — `tar -xzf anchor-*.tar.gz` leaves you a ready-to-run `anchor`. Because the builds are unsigned, macOS quarantines the binary on first run; clear it with `xattr -dr com.apple.quarantine anchor`.

### Build from source

1. [Install Go](https://go.dev/doc/install)

2. Git clone this repository:
```sh
git clone https://github.com/garrettjoecox/anchor.git && cd anchor
```

3. Run the server:
```sh
go run .
```

### Docker

```sh
docker run -p 43383:43383 -v /my/mnt/logs:/app/logs ghcr.io/garrettjoecox/anchor:latest
```

Optional environment variables can be set:

- `PORT`: configures the server port inside the container; defaults to `43383`
- `Volumes`: mounts a local directory to a directory in the container; our example uses the log folder

### Docker Compose
[Example docker compose file](/compose.yml) 
```sh
docker compose up -d
```

Any configurable environment variables can be viewed [here](#docker).
