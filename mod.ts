import { writeAll } from "https://deno.land/std@0.192.0/streams/write_all.ts";
import { readLines } from "https://deno.land/std@0.193.0/io/read_lines.ts";

const decoder = new TextDecoder();
const encoder = new TextEncoder();

type ClientData = Record<string, any>;

interface BasePacket {
  clientId?: number;
  roomId?: string;
  quiet?: boolean;
  targetClientId?: number;
}

interface UpdateClientDataPacket extends BasePacket {
  type: "UPDATE_CLIENT_DATA";
  data: ClientData;
}

interface AllClientDataPacket extends BasePacket {
  type: "ALL_CLIENT_DATA";
  clients: ClientData[];
}

interface ServerMessagePacket extends BasePacket {
  type: "SERVER_MESSAGE";
  message: string;
}

interface DisableAnchorPacket extends BasePacket {
  type: "DISABLE_ANCHOR";
}

interface OtherPackets extends BasePacket {
  type: "REQUEST_SAVE_STATE" | "PUSH_SAVE_STATE";
}

type Packet =
  | UpdateClientDataPacket
  | DisableAnchorPacket
  | ServerMessagePacket
  | AllClientDataPacket
  | OtherPackets;

class Server {
  private listener?: Deno.Listener;
  public clients: Client[] = [];
  public rooms: Room[] = [];

  async start() {
    var mPort = 43384;
    if (Number.isInteger(parseInt(Deno.args[0]))) {
      mPort = parseInt(Deno.args[0]);
    }
    this.listener = Deno.listen({ port: mPort });
    this.log("Server Started On Port "+ mPort);
    try {
      for await (const connection of this.listener) {
        try {
          const client = new Client(connection, this);
          this.clients.push(client);
        } catch (error) {
          this.log(`Error connecting client: ${error.message}`);
        }
      }
    } catch (error) {
      this.log(`Error starting server: ${error.message}`);
    }
  }

  removeClient(client: Client) {
    const index = this.clients.indexOf(client);
    this.clients.splice(index, 1);
  }

  getOrCreateRoom(id: string) {
    const room = this.rooms.find((room) => room.id === id);
    if (room) {
      return room;
    }

    const newRoom = new Room(id, this);
    this.rooms.push(newRoom);
    return newRoom;
  }

  removeRoom(room: Room) {
    const index = this.rooms.indexOf(room);
    this.rooms.splice(index, 1);
  }

  log(message: string) {
    console.log(`[Server]: ${message}`);
  }
}

class Client {
  public id: number;
  public data: ClientData = {};
  private connection: Deno.Conn;
  public server: Server;
  public room?: Room;

  constructor(connection: Deno.Conn, server: Server) {
    this.connection = connection;
    this.server = server;
    this.id = connection.rid;

    this.log("Connected");
    this.waitForData();
  }

  async waitForData() {
    const buffer = new Uint8Array(1024);
    let data = new Uint8Array(0);

    while (true) {
      let count: null | number = 0;

      try {
        count = await this.connection.read(buffer);
      } catch (error) {
        this.log(`Error reading from connection: ${error.message}`);
        this.disconnect();
        break;
      }

      if (!count) {
        this.disconnect();
        break;
      }

      // Concatenate received data with the existing data
      const receivedData = buffer.subarray(0, count);
      data = concatUint8Arrays(data, receivedData);

      // Handle all complete packets (while loop in case multiple packets were received at once)
      while (true) {
        const delimiterIndex = findDelimiterIndex(data);
        if (delimiterIndex === -1) {
          break; // Incomplete packet, wait for more data
        }

        // Extract the packet
        const packet = data.subarray(0, delimiterIndex + 1);
        data = data.subarray(delimiterIndex + 1);

        this.handlePacket(packet);
      }
    }
  }

  handlePacket(packet: Uint8Array) {
    try {
      const packetString = decoder.decode(packet);
      const packetObject: Packet = JSON.parse(packetString);
      packetObject.clientId = this.id;

      if (!packetObject.quiet) {
        this.log(`-> ${packetObject.type} packet`);
      }

      if (packetObject.type === "UPDATE_CLIENT_DATA") {
        this.data = packetObject.data;
      }

      if (packetObject.roomId && !this.room) {
        this.server.getOrCreateRoom(packetObject.roomId).addClient(this);
      }

      if (!this.room) {
        this.log("Not in a room, ignoring packet");
        return;
      }

      if (packetObject.targetClientId) {
        const targetClient = this.room.clients.find((client) =>
          client.id === packetObject.targetClientId
        );
        if (targetClient) {
          targetClient.sendPacket(packetObject);
        } else {
          this.log(`Target client ${packetObject.targetClientId} not found`);
        }
        return;
      }

      if (packetObject.type === "REQUEST_SAVE_STATE") {
        if (this.room.clients.length > 1) {
          this.room.requestingStateClients.push(this);
          this.room.broadcastPacket(packetObject, this);
        }
      } else if (packetObject.type === "PUSH_SAVE_STATE") {
        const roomStateRequests = this.room.requestingStateClients;
        roomStateRequests.forEach((client) => {
          client.sendPacket(packetObject);
        });
        this.room.requestingStateClients = [];
      } else {
        this.room.broadcastPacket(packetObject, this);
      }
    } catch (error) {
      this.log(`Error handling packet: ${error.message}`);
    }
  }

  async sendPacket(packetObject: Packet) {
    try {
      if (!packetObject.quiet) {
        this.log(`<- ${packetObject.type} packet`);
      }
      const packetString = JSON.stringify(packetObject);
      const packet = encoder.encode(packetString + "\n");

      await writeAll(this.connection, packet);
    } catch (error) {
      this.log(`Error sending packet: ${error.message}`);
      this.disconnect();
    }
  }

  disconnect() {
    try {
      if (this.room) {
        this.room.removeClient(this);
      }
      this.server.removeClient(this);
      this.connection.close();
    } catch (error) {
      this.log(`Error disconnecting: ${error.message}`);
    } finally {
      this.log("Disconnected");
    }
  }

  log(message: string) {
    console.log(`[Client ${this.id}]: ${message}`);
  }
}

class Room {
  public id: string;
  public server: Server;
  public clients: Client[] = [];
  public requestingStateClients: Client[] = [];

  constructor(id: string, server: Server) {
    this.id = id;
    this.server = server;
    this.log("Created");
  }

  addClient(client: Client) {
    this.log(`Adding client ${client.id}`);
    this.clients.push(client);
    client.room = this;

    this.broadcastAllClientData();
  }

  removeClient(client: Client) {
    this.log(`Removing client ${client.id}`);
    const index = this.clients.indexOf(client);
    this.clients.splice(index, 1);
    client.room = undefined;

    if (this.clients.length) {
      this.broadcastAllClientData();
    } else {
      this.log("No clients left, removing room");
      this.server.removeRoom(this);
    }
  }

  broadcastAllClientData() {
    this.log("<- ALL_CLIENT_DATA packet");
    for (const client of this.clients) {
      const packetObject = {
        type: "ALL_CLIENT_DATA" as const,
        roomId: this.id,
        clients: this.clients.filter((c) => c !== client).map((c) => ({
          clientId: c.id,
          ...c.data,
        })),
      };

      client.sendPacket(packetObject);
    }
  }

  broadcastPacket(packetObject: Packet, sender: Client) {
    if (!packetObject.quiet) {
      this.log(`<- ${packetObject.type} packet from ${sender.id}`);
    }

    for (const client of this.clients) {
      if (client !== sender) {
        client.sendPacket(packetObject);
      }
    }
  }

  log(message: string) {
    console.log(`[Room ${this.id}]: ${message}`);
  }
}

function concatUint8Arrays(a: Uint8Array, b: Uint8Array): Uint8Array {
  const result = new Uint8Array(a.length + b.length);
  result.set(a, 0);
  result.set(b, a.length);
  return result;
}

function findDelimiterIndex(data: Uint8Array): number {
  for (let i = 0; i < data.length; i++) {
    if (data[i] === 10 /* newline character */) {
      return i;
    }
  }
  return -1;
}

const server = new Server();
server.start();

function sendServerMessage(client: Client, message: string) {
  return client.sendPacket({
    type: "SERVER_MESSAGE",
    message,
  });
}

function sendDisable(client: Client, message: string) {
  sendServerMessage(client, message)
    .finally(() =>
      client.sendPacket({
        type: "DISABLE_ANCHOR",
      })
    );
}

async function stop(message = "Server restarting") {
  await Promise.all(
    server.clients.map((client) =>
      sendServerMessage(client, message)
        .finally(() => {
          client.disconnect();
        })
    ),
  );

  Deno.exit();
}

(async () => {
  try {
    for await (const line of readLines(Deno.stdin)) {
      const [command, ...args] = line.split(" ");

      switch (command) {
        default:
        case "help": {
          console.log(
            `Available commands:
  help: Show this help message
  roomCount: Show the number of rooms
  clientCount: Show the number of clients
  list: List all rooms and clients
  stop <message>: Stop the server
  message <clientId> <message>: Send a message to a client
  messageAll <message>: Send a message to all clients
  disable <clientId> <message>: Disable anchor on a client
  disableAll <message>: Disable anchor on all clients`,
          );
          break;
        }
        case "roomCount": {
          console.log(`Room count: ${server.rooms.length}`);
          break;
        }
        case "clientCount": {
          console.log(`Client count: ${server.clients.length}`);
          break;
        }
        case "list": {
          for (const room of server.rooms) {
            console.log(`Room ${room.id}:`);
            for (const client of room.clients) {
              console.log(
                `  Client ${client.id}: ${JSON.stringify(client.data)}`,
              );
            }
          }
          break;
        }
        case "disable": {
          const [clientId, ...messageParts] = args;
          const message = messageParts.join(" ");
          const client = server.clients.find((c) =>
            c.id === parseInt(clientId, 10)
          );
          if (client) {
            sendDisable(client, message);
          } else {
            console.log(`Client ${clientId} not found`);
          }
          break;
        }
        case "disableAll": {
          const message = args.join(" ");
          for (const client of server.clients) {
            sendDisable(client, message);
          }
          break;
        }
        case "message": {
          const [clientId, ...messageParts] = args;
          const message = messageParts.join(" ");
          const client = server.clients.find((c) =>
            c.id === parseInt(clientId, 10)
          );
          if (client) {
            sendServerMessage(client, message);
          } else {
            console.log(`Client ${clientId} not found`);
          }
          break;
        }
        case "messageAll": {
          const message = args.join(" ");
          for (const client of server.clients) {
            sendServerMessage(client, message);
          }
          break;
        }
        case "stop": {
          const message = args.join(" ");
          stop(message);
          break;
        }
      }
    }
  } catch (error) {
    console.error("Error readingt from stdin: ", error.message);
  }
})();
