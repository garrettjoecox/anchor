![anchor](https://github.com/garrettjoecox/OOT/assets/7316699/a8feac51-47b6-4e4c-b940-2f49fc0bc764)

## What is this?

Anchor is a mod+server that enables co-op on Ship of Harkinian. It's primary
functions are loading save state from a remote player when you join their
session, and sending flag sets/item gives across all players in a session. This
is not multi-world, and it requires that all players have the same randomizer
seed loaded, but this is a great example to reference for it and other
multiplayer ventures.

## How to

You don't need this, I'm hosting this on my end. Instead see
[the associated SoH Build](https://github.com/garrettjoecox/OOT/pull/52)

## Packet protocol

> This is for anyone wanting to extend the client side of anchor while still
> using the hosted server

```ts
// Packets that the client will receive from server
interface IncomingPacket {
  type: string;
  roomId: string; // roomId which the client belongs to
  clientId?: number; // clientId whom the packet came from. Server can send packets so not always provided
  quiet?: boolean; // prevent this packet from logging. Any position/location packets should use this
  ...any valid json
}
// Packets the client sends to server
interface OutgoingPacket {
  type: string;
  roomId: string; // roomId which the client belongs to
  targetClientId?: number; // the server will only send this packet to the targetted client ID
  quiet?: boolean; // prevent this packet from logging. Any position/location packets should use this
  ...any valid json
}
```

All packets sent to server will be forwarded to all clients in the same room
with the following exceptions:

- If the packet contains a `targetClientId` it will only be forwarded to that
  one client
- If the packet is `PUSH_SAVE_STATE`, it will only be sent to clients who have
  requested a save state with `REQUEST_SAVE_STATE`

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
