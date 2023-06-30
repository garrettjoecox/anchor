const listener = Deno.listen({ port: 43384 });

const connections: Deno.Conn[] = [];

for await (const connection of listener) {
  console.log('SoH Client Connected');
  connections.push(connection);
  handleConnection(connection)
    .catch((err) => {
      console.error(err);
      connection.close();
      const index = connections.indexOf(connection);
      connections.splice(index, 1);
    });
}

async function handleConnection(connection: Deno.Conn) {
  let buffer = new Uint8Array(1024);
  let data = new Uint8Array(0);

  while (true) {
    const count = await connection.read(buffer);
    if (!count) {
      // connection closed
      const index = connections.indexOf(connection);
      connections.splice(index, 1);
      console.log('Lost connection to SoH Client');
      break;
    }

    // Concatenate received data with the existing data
    const receivedData = buffer.subarray(0, count);
    data = concatUint8Arrays(data, receivedData);

    while (true) {
      const delimiterIndex = findDelimiterIndex(data);
      if (delimiterIndex === -1) {
        break; // Incomplete packet, wait for more data
      }

      // Extract the packet
      const packet = data.subarray(0, delimiterIndex + 1);
      data = data.subarray(delimiterIndex + 1);

      console.log('Received packet:', new TextDecoder().decode(packet));

      // Forward the packet to other clients
      for (const currentConnection of connections) {
        if (currentConnection !== connection) {
          await currentConnection.write(packet);
        }
      }
    }
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