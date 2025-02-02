![anchor](https://github.com/garrettjoecox/OOT/assets/7316699/a8feac51-47b6-4e4c-b940-2f49fc0bc764)

## What is this?

Anchor is a client/server service for providing multiplayer functions in Harbor Masters 64 ports. It's primary functions are loading save state from a remote player when you join their session, and sending flag sets/item gives across all players in a room/team. 

This implementation of a client/server model is very generic, allowing for multiple games to use it's functions at once as the client software is responsible for handling all of the game state.

## How to

You don't technically need this as there is a public server available that is the default in all HM64 ports.

Here is a list of supported client software developed by the HM64 team:
- [Ship of Harkinian](https://github.com/HarbourMasters/Shipwright/pull/4910)
- [2 Ship 2 Harkinian]()

### Run the server
If you would like to host your own server:

1. [Install Go](https://go.dev/doc/install)

2.  Clone the repo:
  https://github.com/garrettjoecox/anchor and run it with one of the following:

#### Quickly compile and run
```
go run .
```

#### Compile a binary
You can compile the program into a binary by running this command
```sh
go build .
```

### Docker

```sh
docker run -p 43383:43383 -v /my/mnt/logs:/app/logs ghcr.io/garrettjoecox/anchor:latest
```

Optional environment variables can be set:

- `PORT`: configures the server port inside the container; defaults to `43383`
- `Volumes`: mounts a local directory to a directory in the container; our example uses the log folder

### Docker Compose
We also have an example [docker compose file](/compose.yml) 
```sh
docker compose up -d
```

Any configurable environment variables can be viewed [here](#docker).

## Packet protocol

> This is for anyone wanting to extend the client side of anchor while still
> using the hosted server

Packets are delimited by a null terminator `\0`. Clients built prior to December
5th 2023 instead use a newline `\n` as a delimiter, if you are using one of
these clients you will need to use the `legacy-newline-terminator` branch of
this repo.

```json
// Packets that the client will receive from server
{
  "type": "string",
  "roomId": "string", // roomId which the client belongs to
  "clientId": "number", // clientId whom the packet came from. Server can send packets so not always provided
  "quiet": "boolean", // prevent this packet from logging. Any position/location packets should use this
  ...any valid json
},
// Packets the client sends to server
{
  "type": "string",
  "roomId": "string", // roomId which the client belongs to
  "targetClientId": "number", // the server will only send this packet to the targetted client ID
  "quiet": "boolean", // prevent this packet from logging. Any position/location packets should use this
  ...any valid json
}
```

All packets sent to server will be forwarded to all clients in the same room
with the following exceptions:

- If the packet contains a `targetClientId` it will only be forwarded to that
  one client
- If the packet is `PUSH_SAVE_STATE`, it will only be sent to clients who have
  requested a save state with `REQUEST_SAVE_STATE`
- If the packet contains a `targetteamId` it will only be forwarded to that team in the room

Upon joining a room a client should register it's `data` with the
`UPDATE_CLIENT_DATA` packet. The data should be an object with string keys and
arbitrary values, for example:

```json
{
  "type": "UPDATE_CLIENT_DATA",
  "roomId": "testRoom",
  "data": {
    "name": "ProxySaw",
    "color": { "r": 0, "g": 255, "b": 0 }
  }
}
```

Upon any client joining or leaving a room, an `ALL_CLIENT_DATA` packet is sent
to all clients with the remaining client's registered `data` for example:

```json
{
  "type": "ALL_CLIENT_DATA",
  "roomId": "testRoom",
  "clients": [
    {
      "clientId": 45,
      "name": "ProxySaw",
      "color": { "r": 0, "g": 255, "b": 0 }
    }
  ]
}
```

To clarify, it is up to the clients to send/parse this `data`, so this can be
anything you might want to store.
