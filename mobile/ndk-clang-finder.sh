#!/bin/bash

# Check if ANDROID_HOME is set
if [ -z "$ANDROID_HOME" ]; then
    echo "Error: ANDROID_HOME environment variable is not set." >&2
    exit 1
fi

# Remove trailing slash from ANDROID_HOME if present
ANDROID_HOME="${ANDROID_HOME%/}"

# Find latest NDK version
NDK_DIR="$ANDROID_HOME/ndk"
LATEST_NDK=$(ls -v "$NDK_DIR" | tail -n1)

if [ -z "$LATEST_NDK" ]; then
    echo "Error: No NDK version found in $NDK_DIR" >&2
    exit 1
fi

# Determine the host OS
case "$(uname -s)" in
    Darwin)
        HOST_OS="darwin-x86_64"
        ;;
    Linux)
        HOST_OS="linux-x86_64" # TODO needs to be tested
        ;;
    MINGW*|MSYS*)
        HOST_OS="windows-x86_64" # TODO needs to be tested
        ;;
    *)
        echo "Error: Unsupported operating system" >&2
        exit 1
        ;;
esac

# Construct the base path for the clang compilers
CLANG_DIR="$ANDROID_HOME/ndk/$LATEST_NDK/toolchains/llvm/prebuilt/$HOST_OS/bin"

# Map GOARCH to Android architecture
case "$GOARCH" in
    arm64)
        ANDROID_ARCH="aarch64-linux-android"
        ;;
    arm)
        ANDROID_ARCH="armv7a-linux-androideabi"
        ;;
    386)
        ANDROID_ARCH="i686-linux-android"
        ;;
    amd64)
        ANDROID_ARCH="x86_64-linux-android"
        ;;
    *)
        echo "Error: Unsupported architecture $GOARCH" >&2
        exit 1
        ;;
esac

# Find the latest API level for the specific architecture
LATEST_API=$(ls "$CLANG_DIR" | grep "^$ANDROID_ARCH" | grep -Eo "$ANDROID_ARCH[0-9]+" | sed "s/$ANDROID_ARCH//" | sort -rn | head -1)

if [ -z "$LATEST_API" ]; then
    echo "Error: Could not determine the latest API level for $ANDROID_ARCH" >&2
    exit 1
fi

# Output the path to the appropriate clang compiler
echo $CLANG_DIR/$ANDROID_ARCH$LATEST_API-clang
