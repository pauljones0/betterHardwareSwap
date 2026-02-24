# Requirements Document

## Functional Requirements

### 1. Reddit Ingestion
*   **Req 1.1**: The system must fetch the latest posts from `r/CanadianHardwareSwap` at an interval no longer than 60 seconds.
*   **Req 1.2**: The system must use the unofficial Reddit `.json` endpoint.
*   **Req 1.3**: The system must employ exponential backoff and retry logic when communicating with external dependencies (Reddit and Gemini AI) to handle rate limits, transient errors, or service interruptions.

### 2. User Alert Management
*   **Req 2.1**: Users must be able to define alerts using natural language via a Discord slash command (e.g., `/alert wtb RTX 3080`).
*   **Req 2.2**: The system must process natural language requests using an AI model to generate optimized boolean search queries.
*   **Req 2.3**: If an alert is too broad (e.g., `/alert mouse`), the system must warn the user before saving it.

### 3. AI Evaluation & Deal Filtering
*   **Req 3.1**: Every new post must be evaluated against all active user alerts by the AI model.
*   **Req 3.2**: The AI must extract relevant metadata including price, location, and the summarized item description.

### 4. Discord Integration
*   **Req 4.1**: When a post matches one or more alerts, the system must consolidate the pings into a single, clean Discord embed message.
*   **Req 4.2**: The Discord embed must strip out Reddit-specific jargon (like `[H]`, `[W]`, `[Local]`) and present the data cleanly for mobile readability.
*   **Req 4.3**: The system must retroactively update previously sent Discord messages when the original Reddit post flair changes to `Sold` or `Closed`, striking through the message text.

## Non-Functional Requirements

### 1. Serverless Scale & Cost
*   **Req 5.1**: The application must be 100% serverless, scaling to zero compute instances when not actively processing a scrape or interaction.
*   **Req 5.2**: The data store must support a pay-per-operation model (e.g., Firestore Native Mode) without persistent hourly costs.

### 2. Observability & Maintainability
*   **Req 6.1**: The system must output structured JSON logs capable of tracing a single request across the interaction, scraping, and Discord webhook bounds using a `request_id`.
*   **Req 6.2**: The data store must proactively trim (garbage collect) post records older than the 500 most recent items.
*   **Req 6.3**: The processing pipeline must utilize parallel execution to ensure all new posts are evaluated within the 60-second cron window even as the number of active alerts grows.

### 3. Multi-Tenancy
*   **Req 7.1**: The architecture must support the bot being installed in multiple Discord servers simultaneously, keeping channel routing boundaries distinct.

## Security Requirements

*   **Req 8.1**: The system must verify the cryptographic signature of every incoming Discord interaction.
*   **Req 8.2**: The application container must run as a non-root user.
*   **Req 8.3**: All AI prompts must include headers to mitigate instruction injection attacks.
