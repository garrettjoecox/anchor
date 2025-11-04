import { $ } from "bun";
import { Client, GatewayIntentBits, PresenceUpdateStatus } from "discord.js";

if (!Bun.env.TOKEN || !Bun.env.RESTART_CHANNEL || !Bun.env.LFG_CHANNEL || !Bun.env.LFG_INFO_MESSAGE || !Bun.env.RESTART_MENTION) {
  throw new Error("Missing required environment variables");
}

enum ActivityState {
  OnlinePlayers,
  UniquePlayers,
  GamesCompleted,
}

let anchorOnline = false;
let restarting = false;
let activitiyState = ActivityState.OnlinePlayers;

const discordClient = new Client({ intents: [GatewayIntentBits.Guilds, GatewayIntentBits.GuildPresences] });

await discordClient.login(process.env.TOKEN);

let stats = {
  lastStatsHeartbeat: 0,
  uniqueCount: 0,
  onlineCount: 0,
  gameCompleteCount: 0,
  pid: 0,
};

async function refreshStats() {
  try {
    stats = await Bun.file("./stats.json").json();
  } catch (error) {
    console.error("An error occured while reading stats.json", error);
  }

  setTimeout(refreshStats, 1000 * 5);
};

// Await the first one
await refreshStats();

(async function refreshStatus() {
  activitiyState += 1;
  if (activitiyState > ActivityState.GamesCompleted) {
    activitiyState = ActivityState.OnlinePlayers;
  }

  try {
    let status: PresenceUpdateStatus = PresenceUpdateStatus.Online;
    let activity = "";
    switch (activitiyState) {
      case ActivityState.OnlinePlayers:
        activity = `/ ${stats.onlineCount} Online Now`;
        break;
      case ActivityState.UniquePlayers:
        activity = `/ ${stats.uniqueCount} Unique Players`;
        break;
      case ActivityState.GamesCompleted:
      default:
        activity = `/ ${stats.gameCompleteCount} Games Complete`;
        break;
    }

    if (restarting) {
      status = PresenceUpdateStatus.Idle;
      activity = "/ Restarting";
    } else if (!anchorOnline) {
      status = PresenceUpdateStatus.DoNotDisturb;
      activity = "/ Offline";
    }

    discordClient!.user?.setPresence({ activities: [{ name: activity, type: 0 }], status });
  } catch (error) {
    console.error("An error occured while refreshing the bot status", error);
  }

  setTimeout(refreshStatus, 1000 * 10);
})();

(async function autoRestart() {
  try {
    // Heartbeat occured in last 60 seconds
    if (stats.lastStatsHeartbeat > Date.now() - 1000 * 60) {
      if (restarting) {
        console.log("Server is back online");
        try {
          const channel = await discordClient!.channels.fetch(Bun.env.RESTART_CHANNEL!);
          if (channel?.isSendable()) {
            await channel.send("Anchor server back online!");
          }
        } catch (error) {
          console.error("An error occured while notifying of restart", error);
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

    try {
      let message = "Anchor server is down, attempting to restart...";
      if (Bun.env.RESTART_MENTION) {
        message += ` CC <@${Bun.env.RESTART_MENTION}>`;
      }

      const channel = discordClient!.channels.cache.get(Bun.env.RESTART_CHANNEL!);
      if (channel?.isSendable()) {
        await channel.send(message);
      }
    } catch (error) {
      console.error("An error occured while notifying of restart", error);
    }

    try {
      // Write screen config so that it uses a fresh log file
      await Bun.write(
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
        await $`kill ${stats.pid}`.text();
      } catch (_) {
        console.log("Failed to kill server, probably already dead");
      }
    }

    await Bun.spawn(['screen', '-c', './.screenrc', '-dmLS', 'anchor-go', 'go', 'run', '.']);
  } catch (error) {
    console.error("An error occured while attempting to restart server", error);
  }

  setTimeout(autoRestart, 1000 * 10);
})();

// This channel is made up of messages that have thread channels associated with them
// If a message has not been sent in the last 6 hours, the thread channel is deleted
(async function pruneLFGChannel() {
  try {
    const channel = await discordClient!.channels.fetch(Bun.env.LFG_CHANNEL!);
    if (!channel) {
      console.warn("LFG channel does not exist, not beginning pruning process");
      return;
    }

    if (!channel.isTextBased()) {
      return;
    }

    const lfgMessages = await channel.messages.fetch({ limit: 100 });

    lfgMessages.forEach(async (lfgMessage) => {
      // Ignore messages that are not 5 minutes old yet or that are the info message
      if (
        lfgMessage.createdTimestamp > Date.now() - 1000 * 60 * 5 ||
        lfgMessage.id.toString() === Bun.env.LFG_INFO_MESSAGE
      ) {
        return;
      }

      // If the user has not created the thread yet, delete the message
      if (!lfgMessage.thread?.id) {
        await lfgMessage.delete();
        return;
      }

      // fetch the last 2 messages in the thread
      const threadMessages = await lfgMessage.thread.messages.fetch({ limit: 2 });

      // Filter out messages from the bot
      const lastThreadMessage = threadMessages.find((m) =>
        m.author.id !== discordClient!.user!.id
      );

      // If there is no message, or the last message was sent more than 6 hours ago, delete the original message and thread
      if (
        !lastThreadMessage ||
        lastThreadMessage.createdTimestamp < Date.now() - 1000 * 60 * 60 * 6
      ) {
        await lfgMessage.thread!.delete();
        await lfgMessage.delete();
        return;
      }
    });
  } catch (error) {
    console.error("An error occured while pruning LFG channel", error);
  }

  setTimeout(pruneLFGChannel, 1000 * 60);
})();
