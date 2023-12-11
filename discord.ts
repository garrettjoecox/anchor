import { load } from "https://deno.land/std@0.208.0/dotenv/mod.ts";
import {
  Bot,
  createBot,
  Intents,
  PresenceStatus,
} from "npm:@discordeno/bot@19.0.0-next.8b3bc4b";

interface ServerStats {
  lastStatsHeartbeat: number;
  clientSHAs: Record<string, boolean>;
  onlineCount: number;
  gamesCompleted: number;
  pid: number;
}

enum ActivityState {
  OnlinePlayers,
  UniquePlayers,
  GamesCompleted,
}

const env = await load();

let botReady = false;
let anchorOnline = false;
let restarting = false;
let bot: Bot;
let activtiyState = ActivityState.OnlinePlayers;
let stats: ServerStats = {
  lastStatsHeartbeat: 0,
  clientSHAs: {},
  onlineCount: 0,
  gamesCompleted: 0,
  pid: 0,
};

(async () => {
  try {
    if (!env.TOKEN) {
      console.warn("No bot token provided, continuing without bot");
      return;
    }

    bot = createBot({
      token: env.TOKEN,
      intents: Intents.GuildPresences | Intents.Guilds,
      events: {
        ready: () => {
          console.log("Bot online");
          botReady = true;
        },
      },
    });

    await bot.start();
  } catch (error) {
    console.warn("An error occured while starting the bot", error);
    console.warn("Continuing without bot functionality");
  }
})();

(async function refreshStats() {
  try {
    const statsString = await Deno.readTextFile("./stats.json");
    stats = JSON.parse(statsString);
  } catch (error) {
    console.error("An error occured while reading stats.json", error);
  }

  setTimeout(refreshStats, 1000 * 5);
})();

(async function refreshStatus() {
  activtiyState += 1;
  if (activtiyState > ActivityState.GamesCompleted) {
    activtiyState = ActivityState.OnlinePlayers;
  }

  if (!botReady) {
    return setTimeout(refreshStatus, 1000 * 10);
  }

  try {
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

    await bot!.gateway.editBotStatus({
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
    // Heartbeat occured in last 30 seconds
    if (stats.lastStatsHeartbeat > Date.now() - 1000 * 30) {
      if (restarting) {
        console.log("Server is back online");
        if (botReady && env.RESTART_CHANNEL) {
          try {
            bot!.helpers.sendMessage(env.RESTART_CHANNEL, {
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

        bot!.helpers.sendMessage(env.RESTART_CHANNEL, {
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
deflog on
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
