FROM denoland/deno:latest

# The port that your application listens to.
EXPOSE 43384

WORKDIR /app

# Prefer not to run as root.
USER deno

CMD ["run", "--allow-all", "https://raw.githubusercontent.com/garrettjoecox/anchor/main/mod.ts"]
