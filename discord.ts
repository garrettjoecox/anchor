import { load } from "https://x.nest.land/Yenv@1.0.0/mod.ts";
import {
  createBot,
  Intents,
  PresenceStatus,
} from "npm:@discordeno/bot@19.0.0-next.8b3bc4b";

interface ServerStats {
  lastHeartbeat: number;
  clientSHAs: Record<string, boolean>;
  onlineCount: number;
  gamesCompleted: number;
  pid: number;
}

const env = await load({
  TOKEN: /.+/,
  RESTART_CHANNEL: /.+/,
  RESTART_MENTION: /.+/,
});

let botReady = false;
let anchorOnline = false;
let restarting = false;

const bot = createBot({
  token: env.TOKEN,
  intents: Intents.GuildPresences | Intents.Guilds,
  events: {
    ready: () => {
      console.log("Bot online");
      botReady = true;
    },
  },
});

(async () => {
  try {
    await bot.start();
  } catch (error) {
    console.error("An error occured while starting the bot", error);
  }
})();

enum ActivityState {
  OnlinePlayers,
  UniquePlayers,
  GamesCompleted,
}

let activtiyState = ActivityState.OnlinePlayers;

(async function refreshStatus() {
  activtiyState += 1;
  if (activtiyState > ActivityState.GamesCompleted) {
    activtiyState = ActivityState.OnlinePlayers;
  }

  if (!botReady) {
    return setTimeout(refreshStatus, 1000 * 10);
  }

  try {
    const statsString = await Deno.readTextFile("./stats.json");
    const stats: ServerStats = JSON.parse(statsString);

    let status: keyof typeof PresenceStatus = "online";
    let activtiy = "";
    switch (activtiyState) {
      case ActivityState.OnlinePlayers:
        activtiy = `/ ${stats.onlineCount} Online Now`;
        break;
      case ActivityState.UniquePlayers:
        activtiy = `/ ${Object.keys(stats.clientSHAs).length} Unique Players`;
        break;
      case ActivityState.GamesCompleted:
      default:
        activtiy = `/ ${stats.gamesCompleted} Games Complete`;
        break;
    }

    if (restarting) {
      status = "idle";
      activtiy = "/ Restarting";
    } else if (!anchorOnline) {
      status = "dnd";
      activtiy = "/ Offline";
    }

    await bot.gateway.editBotStatus({
      status: status,
      activities: [
        {
          name: activtiy,
          type: 0,
        },
      ],
    });
  } catch (error) {
    console.error("An error occured while refreshing the bot status", error);
  }

  setTimeout(refreshStatus, 1000 * 10);
})();

(async function autoRestart() {
  try {
    const statsString = await Deno.readTextFile("./stats.json");
    const stats: ServerStats = JSON.parse(statsString);

    // Heartbeat occured in last 2 minutes (4 heartbeats)
    if (stats.lastHeartbeat > Date.now() - 1000 * 60 * 2) {
      if (restarting) {
        console.log("Server is back online");
        if (botReady && env.RESTART_CHANNEL) {
          try {
            bot.helpers.sendMessage(env.RESTART_CHANNEL, {
              content: "Anchor server back online!",
            });
          } catch (error) {
            console.error("An error occured while notifying of restart", error);
          }
        }
      }
      anchorOnline = true;
      restarting = false;
      return setTimeout(autoRestart, 1000 * 10);
    }

    if (restarting) {
      console.log("Server is restarting, waiting for it to come back up");
      return setTimeout(autoRestart, 1000 * 10);
    }

    anchorOnline = false;
    restarting = true;
    console.log("Server is down, notifying and restarting");

    if (botReady && env.RESTART_CHANNEL) {
      try {
        let message = "Anchor server is down, attempting to restart...";
        if (env.RESTART_MENTION) {
          message += ` CC <@${env.RESTART_MENTION}>`;
        }

        bot.helpers.sendMessage(env.RESTART_CHANNEL, {
          content: message,
        });
      } catch (error) {
        console.error("An error occured while notifying of restart", error);
      }
    }

    try {
      // Write screen config so that it uses a fresh log file
      await Deno.writeTextFile(
        "./.screenrc",
        `logfile "logs/${
          new Date().toLocaleString().replace(/[\s,:/]/g, "-")
        }.log"
        logfile flush 1`,
      );
    } catch (error) {
      console.error("An error occured while writing screen config", error);
    }

    if (stats.pid) {
      try {
        Deno.kill(stats.pid, "SIGINT");
      } catch (_) {
        console.log("Failed to kill server, probably already dead");
      }
    }

    const command = new Deno.Command(
      "screen",
      {
        args: [
          "-c",
          "./.screenrc",
          "-dmLS",
          "anchor",
          "deno",
          "run",
          "--allow-all",
          "mod.ts",
        ],
        env: {
          QUIET: "TRUE",
        },
        stdout: "null",
        stderr: "null",
        stdin: "null",
      },
    );
    const process = command.spawn();
    process.unref();
  } catch (error) {
    console.error("An error occured while attempting to restart server", error);
  }

  setTimeout(autoRestart, 1000 * 10);
})();
