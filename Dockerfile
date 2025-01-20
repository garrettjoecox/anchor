FROM golang:alpine

# The port that your application listens to.
EXPOSE 43383

WORKDIR /app

# Copy in source code
COPY *.go *.sum *.mod .

RUN mkdir -p /app/logs

# Compile the app
RUN go build -o bin .

#remove the go files after compiling
RUN rm *.go *.sum *.mod

ENTRYPOINT ["/app/bin"]