import { connect } from "node:net";

const WEBHOOK_URL = Bun.env.DISCORD_WEBHOOK_URL;
if (!WEBHOOK_URL) {
  console.warn("DISCORD_WEBHOOK_URL is not set, the server will still be restarted but nothing will be posted to Discord");
}

const SERVER_PORT = 43383;
const CHECK_INTERVAL = 1000 * 5;
const FAILED_CHECKS_BEFORE_DOWN = 2;
const START_TIMEOUT = 1000 * 60;

let down = false;
let failedChecks = 0;
let lastStartAttempt = 0;

function isServerUp(): Promise<boolean> {
  return new Promise((resolve) => {
    const socket = connect({ host: "127.0.0.1", port: SERVER_PORT });
    const finish = (up: boolean) => {
      socket.destroy();
      resolve(up);
    };

    socket.setTimeout(1000 * 2, () => finish(false));
    socket.once("connect", () => finish(true));
    socket.once("error", () => finish(false));
  });
}

async function notify(content: string) {
  if (!WEBHOOK_URL) {
    return;
  }

  try {
    const response = await fetch(WEBHOOK_URL, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content }),
      signal: AbortSignal.timeout(1000 * 5),
    });

    if (!response.ok) {
      console.error(`Discord webhook returned ${response.status}`);
    }
  } catch (error) {
    console.error("An error occurred while calling the Discord webhook:", error instanceof Error ? error.message : error);
  }
}

async function startServer() {
  lastStartAttempt = Date.now();

  try {
    await Bun.write(
      "./.screenrc",
      `logfile "logs/${
        new Date().toLocaleString().replace(/[\s,:/]/g, "-")
      }.log"
deflog on
logfile flush 1`,
    );

    Bun.spawn(["screen", "-c", "./.screenrc", "-dmLS", "anchor-go", "go", "run", "."]);
  } catch (error) {
    console.error("An error occurred while attempting to start server", error);
  }
}

async function check() {
  if (await isServerUp()) {
    failedChecks = 0;

    if (down) {
      down = false;
      console.log("Server is back online");
      notify("Anchor server back online!");
    }

    return;
  }

  failedChecks++;

  if (!down) {
    if (failedChecks < FAILED_CHECKS_BEFORE_DOWN) {
      return;
    }

    down = true;
    console.log("Server is down, notifying and restarting");
    await startServer();

    let message = "Anchor server is down, attempting to restart...";
    if (Bun.env.RESTART_MENTION) {
      message += ` CC <@${Bun.env.RESTART_MENTION}>`;
    }
    notify(message);
  } else if (Date.now() - lastStartAttempt > START_TIMEOUT) {
    console.log("Server has not come back up, attempting to restart again");
    await startServer();
  }
}

while (true) {
  try {
    await check();
  } catch (error) {
    console.error("An error occurred while checking on the server", error);
  }

  await Bun.sleep(CHECK_INTERVAL);
}
