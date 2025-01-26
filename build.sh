#!/bin/bash

#build for all platforms
platforms=("windows/amd64" "linux/amd64" "darwin/amd64" "darwin/arm64" "linux/arm64" "windows/arm64")
for platform in "${platforms[@]}"; do
    GOOS=${platform%/*}
    GOARCH=${platform#*/}
    env GOOS=$GOOS GOARCH=$GOARCH go build -o ./bin/anchor-$GOOS-$GOARCH .
done