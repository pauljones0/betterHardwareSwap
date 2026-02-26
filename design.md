# Design Document

## Overall Architecture

**betterHardwareSwap** is a 100% serverless Discord bot built with Go and deployed on Google Cloud Run. It monitors the `r/CanadianHardwareSwap` subreddit for new posts, uses Google Gemini to evaluate those posts against user-defined keywords, and alerts matched users via Discord.

The architecture relies entirely on event-driven execution with no persistent compute running between invocations, allowing it to scale from zero to infinite concurrency as needed.

### Components

1. **Cloud Scheduler (The Pulse)**: Wakes up the bot every minute by making an HTTP GET request to `/cron/scrape`.
2. **Reddit Scraper (`internal/reddit`)**: Fetch the latest 100 posts from `r/CanadianHardwareSwap`'s unofficial `.json` endpoint. Implements exponential backoff (max 3 retries, 10s backoff cap) for high reliability.

   > [!WARNING]
   > **TEMPORARY:** `FetchNewestPosts` is currently stubbed to return an empty feed without making any HTTP requests. GCP Cloud Run egress IPs are being blocked (HTTP 403) by Reddit/Cloudflare. The stub will be removed once OAuth or proxy routing is implemented.

3. **Core Processor (`internal/processor`)**: The orchestrator. Coordinates fetching new Reddit posts, checking existing post statuses, calling the AI to parse post content, identifying matching user alerts, and triggering Discord notifications. Uses **Interfaces** (`DiscordMessenger`, `Scraper`) to decouple core logic from external dependencies, enabling robust unit testing. Implements parallel processing using `errgroup` for high-concurrency throughput.
4. **AI Parser (`internal/ai`)**: Interfaces with the Google Gemini 2.5 Flash Lite API. Converts human-readable hardware requests into optimized Boolean logic and processes Reddit post titles/descriptions to determine relevance. Logic is separated into `gemini.go` (client) and `prompts.go` (templates). Implements transient failure retry logic.
5. **Discord Client (`internal/discord`)**: Handles incoming Slash Command interactions from users (e.g., `/setup`, `/alert`) via webhook, and sends outbound webhook messages/embeds to Discord channels when a hardware match is found. Decomposed into `modals.go` (modal entries), `alerts.go` (alert management), and `components.go` (interaction routing).
6. **Data Store (`internal/store`)**: Interacts with Google Cloud Firestore in native mode. Tracks user configured alerts, routing configurations (Discord server ID to channel ID mappings), and the lifecycle of processed Reddit posts (to prevent duplicate pings and allow for retrospective flair updates like `Sold` or `Closed`).
7. **Structured Logger (`internal/logger`)**: Provides JSON-formatted logs with request-id propagation for end-to-end tracing.
8. **Test Utilities (`internal/testutils`)**: Centralized package for standardized mocks and fixture loading to ensure clean, consistent, and maintainable testing across the entire codebase.

## Data Flow

### The Scrape Cycle
1. Cloud Scheduler hits `GET /cron/scrape`.
2. The Processor instantiates the Store, Discord, Scraper, and AI clients using dependency injection via interfaces.
3. The Scraper pulls `.json` posts from Reddit with automatic exponential backoff on retries.
4. The Store is queried to retrieve all active user alerts and existing post records.
5. For each fetched post (processed in parallel):
   - If the post is **old** and its flair changed to `Closed`/`Sold`, the Processor tells Discord to strike-through the original message for historical tracking.
   - If the post is **new**, it is evaluated against the user alerts by the AI Parser.
   - If there is a match, a clean, summarized embed is crafted and sent to the mapped Discord channel for that server.
6. The new post is recorded in the Store to prevent future redundant pings.
7. Periodically, old posts are trimmed from the Store to maintain low latency and storage costs.

### The Interaction Cycle
1. A Discord user types a slash command (e.g., `/alert wtb RTX 3080 under $500`).
2. Discord POSTs the interaction JSON payload to `https://<cloud-run-url>/interactions`.
3. The Discord Client validates the cryptographic signature.
4. The interaction is processed, potentially involving the AI Parser to optimize the search query.
5. The resulting alert configuration is saved in the Store.

## Security Considerations

1. **Interaction Validation**: All incoming requests to `/interactions` are cryptographically verified using Discord's Ed25519 public key before processing.
2. **AI Guardrails**: The AI prompts include strict anti-injection instructions to prevent users from manipulating the query generation logic.
3. **Container Hardening**: The application runs as a non-root user (`appuser`, UID 10001) in a minimal Alpine-based container to reduce the attack surface.
4. **Credential Isolation**: Local development relies on `.env` files (git-ignored), while GCP deployments use environment variables managed via GitHub Secrets and Cloud Run configuration.
