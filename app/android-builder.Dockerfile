# Android build image for the Parley app shell.
# Build it with scripts/build-apk (Docker in WSL); it is not a product artefact.

# Bun installs the app workspace's JS deps (Tauri CLI) and runs it.
FROM oven/bun:1.4.3 AS bun

FROM rust:1.98-bookworm

COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
# `bunx` is a symlink in the oven/bun image; COPY above takes only the binary.
RUN ln -s /usr/local/bin/bun /usr/local/bin/bunx

ARG CMDLINE_TOOLS=16111833
ARG ANDROID_PLATFORM=android-37.0
ARG ANDROID_BUILD_TOOLS=37.0.0
ARG ANDROID_NDK=30.0.16248370

ENV ANDROID_HOME=/opt/android-sdk \
    ANDROID_SDK_ROOT=/opt/android-sdk \
    NDK_HOME=/opt/android-sdk/ndk/${ANDROID_NDK} \
    JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64 \
    PATH=/opt/android-sdk/cmdline-tools/latest/bin:/opt/android-sdk/platform-tools:${PATH}

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      openjdk-17-jdk-headless curl unzip xz-utils git pkg-config libssl-dev \
 && rm -rf /var/lib/apt/lists/*

RUN mkdir -p "${ANDROID_HOME}/cmdline-tools" \
 && curl -fsSL "https://dl.google.com/android/repository/commandlinetools-linux-${CMDLINE_TOOLS}_latest.zip" -o /tmp/cmdline-tools.zip \
 && unzip -q /tmp/cmdline-tools.zip -d "${ANDROID_HOME}/cmdline-tools" \
 && mv "${ANDROID_HOME}/cmdline-tools/cmdline-tools" "${ANDROID_HOME}/cmdline-tools/latest" \
 && rm /tmp/cmdline-tools.zip

RUN yes | sdkmanager --licenses > /dev/null \
 && sdkmanager --install "platform-tools" "platforms;${ANDROID_PLATFORM}" "build-tools;${ANDROID_BUILD_TOOLS}" "ndk;${ANDROID_NDK}" \
 && sdkmanager --list_installed

# Tauri's Android plugin libraries compile against SDK 36; install it up front so
# every build does not have to auto-download it into the disposable container.
RUN yes | sdkmanager --licenses > /dev/null \
 && sdkmanager --install "platforms;android-36" "build-tools;36.0.0"

RUN rustup target add aarch64-linux-android armv7-linux-androideabi i686-linux-android x86_64-linux-android

WORKDIR /work
