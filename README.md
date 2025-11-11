![anchor](https://github.com/garrettjoecox/OOT/assets/7316699/a8feac51-47b6-4e4c-b940-2f49fc0bc764)

## What is this?

Anchor is a client/server service for providing multiplayer functions in Harbor Masters 64 ports. It's primary functions are loading save state from a remote player when you join their session, and sending flag sets/item gives across all players in a room/team. 

This implementation of a client/server model is very generic, allowing for multiple games to use it's functions at once as the client software is responsible for handling all of the game state.

## How to use this?

> [!NOTE]
> For your typical user, you don't actually need this, as we have a public server hosted for general use at `anchor.hm64.org:43383` which is the default on clients. Self hosting may be a better option for you if you are not based in the US however, as latency may be an issue.

### Precompiled Binaries

- [MacOS (arm64/x86_64)](https://nightly.link/garrettjoecox/anchor/workflows/build-binaries/main/anchor-macOS-arm64.zip)
- [Linux (x86_64)](https://nightly.link/garrettjoecox/anchor/workflows/build-binaries/main/anchor-linux-x64.zip)
- [Windows (x86_64)](https://nightly.link/garrettjoecox/anchor/workflows/build-binaries/main/anchor-windows-x64.zip)

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
