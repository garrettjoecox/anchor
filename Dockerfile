FROM golang:alpine

# The port that your application listens to.
EXPOSE 43383

WORKDIR /app

# Copy in source code
COPY *.go *.sum *.mod .

# Compile the app
RUN go build -o bin .

ENTRYPOINT ["/app/bin"]