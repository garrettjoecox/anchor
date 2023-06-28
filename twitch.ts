import * as TwitchIrc from "https://deno.land/x/twitch_irc@0.10.2/mod.ts";
import Template from "https://deno.land/x/template@v0.1.0/mod.ts";
const tpl = new Template();

const client = new TwitchIrc.Client();

const configString = await Deno.readTextFileSync("./config.json");
if (!configString) {
  throw new Error("No config file found, please create a config.json");
}

let config: {
  channel: string;
  commands: { [index: string]: string | string[] };
};

try {
  config = JSON.parse(configString);
} catch (_error) {
  throw new Error("Failed to parse config.json");
}

let commandQueue: string[] = [];

client.on("privmsg", (event) => {
  let command: string;
  let args: string[];
  console.log(event);
  if (event.raw.tags?.bits) {
    
  } else if (event.raw.tags?.customRewardId) {
    console.log('Redeem Used:', event.raw.tags?.customRewardId);
    command = event.raw.tags?.customRewardId;
    args = event.message.trim().split(' ');
  } else {
    if (!event.message) return;
    const splitMessage = event.message.trim().split(' ');
    command = splitMessage.shift()!;
    args = splitMessage;
  }

  if (config.commands[command]) {
    const argObject = args.reduce<Record<string, unknown>>((result, arg, index) => {
      result[index] = arg;
      return result;
    }, {});
    const unformattedCommands = (Array.isArray(config.commands[command]) ? config.commands[command] : [config.commands[command]]) as string[];
    const formattedCommands = unformattedCommands.map(cmd => tpl.render(cmd, argObject));

    commandQueue = commandQueue.concat(formattedCommands);
  }
});

client.on("open", async () => {
  await client.join(`#${config.channel}`);
  console.log(`Connected to chat for ${config.channel}`);
});

const listener = Deno.listen({ port: 43384 });

const connections: Deno.Conn[] = [];

setInterval(() => {
    if (commandQueue.length) {
    const command = commandQueue.shift();
    console.log('Sending:', command);
    connections.forEach((conn, index) => {
      conn.write(new TextEncoder().encode(command))
        .catch(() => {
          console.log('Lost connection to SoH Client');
          connections.splice(index, 1);
        });
    });
  }
}, 100);

for await (const conn of listener) {
  console.log('SoH Client Connected');
  connections.push(conn);
}