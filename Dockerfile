# ── Build stage: iTaK Agent ──────────────────────────────────────────
FROM golang:1.26-alpine AS agent-builder

RUN apk add --no-cache git

WORKDIR /src/Agent

# Copy full Agent source (remove vendor to force module download)
COPY Agent/ .
RUN rm -rf vendor

# Copy the embedded iTaK Database module (referenced via replace ../Database)
COPY Database/ /src/Database/

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /itakagent ./cmd/itakagent

# ── Build stage: iTaK Browser ────────────────────────────────────────
FROM golang:1.26-alpine AS browser-builder

RUN apk add --no-cache git

# Copy all workspace modules the browser depends on
WORKDIR /src
COPY Core/ ./Core/
COPY Torch/ ./Torch/
COPY Browser/ ./Browser/

WORKDIR /src/Browser
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /gobrowser ./cmd/gobrowser

# ── Runtime stage ────────────────────────────────────────────────────
FROM alpine:3.21

# Chromium + deps for headless browsing
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    chromium \
    nss \
    freetype \
    harfbuzz \
    font-noto-emoji \
    ttf-freefont

# Make Chromium discoverable by chromedp (it searches for these binary names)
RUN ln -sf /usr/bin/chromium-browser /usr/bin/google-chrome \
 && ln -sf /usr/bin/chromium-browser /usr/bin/google-chrome-stable \
 && ln -sf /usr/bin/chromium-browser /usr/bin/chromium

# Disable sandbox (running as root in Docker)
ENV CHROMEDP_NO_SANDBOX=true

WORKDIR /app

# Copy both binaries
COPY --from=agent-builder /itakagent /app/itakagent
COPY --from=browser-builder /gobrowser /app/gobrowser

# Copy Agent web assets (dashboard HTML/CSS/JS)
COPY Agent/web/ /app/web/

# Make gobrowser available on PATH so the agent can shell out to it
ENV PATH="/app:${PATH}"

# Fixed port for stable testing
ENV ITAK_API_PORT=42800
EXPOSE 42800

VOLUME /app/data

ENTRYPOINT ["/app/itakagent"]
CMD ["serve"]
