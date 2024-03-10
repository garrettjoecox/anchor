FROM denoland/deno:alpine

# The port that your application listens to.
EXPOSE 43385

VOLUME [ "/logs" ]

WORKDIR /app

# Symlink stats.json into a volume, as it's hard to mount from the workdir
RUN mkdir /logs && ln -s /logs/stats.json ./stats.json && chown -R deno:deno /logs

# Prefer not to run as root.
USER deno

# Copy in source code
COPY *.ts .

# Compile the main app so that it doesn't need to be compiled each startup/entry.
RUN deno cache mod.ts

CMD ["run", "--allow-net", "--allow-env", "--allow-write", "mod.ts"]
