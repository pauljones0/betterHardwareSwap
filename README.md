# Canadian Hardware Swap Discord Bot

A completely serverless, zero-maintenance Discord bot that monitors `r/CanadianHardwareSwap` and pings users when newly posted items match their AI-powered keyword alerts. 

Powered by Go, deployed on GCP Cloud Run, scaling to infinite concurrency when called, and scaling to 0 (free) when idle. Storage is backed by Firestore. AI parsing is handled by Gemini 2.5 Flash Lite.

## Living Documentation
This project uses a persona-driven development workflow. Its architecture, requirements, and specifications are continuously updated in the following living documents:
* [Design Document](design.md) - Overall system architecture and data flow.
* [Specification Document](spec.md) - Data models and internal/external APIs.
* [Requirements Document](requirements.md) - Functional and non-functional requirements.

## Features
* **AI Keyword Wizard:** Users tell the bot what they want in plain English, and Gemini builds an optimized Boolean query (Must Include, Can Include, Must Exclude) and warns if the alert is too broad.
* **Smart Alerting:** 1 post = 1 message. If 8 users match, they are cleanly pinged in a single message.
* **Deal Feed & UX:** Posts are stripped of Reddit jargon and summarized cleanly for mobile devices. Pricing, Location, and item names are extracted. Post engagements are shown as reactions.
* **Historical Tracking:** When a Reddit user changes their flair to `Closed` or `Sold`, the Discord message is updated, turned grey, and struck-through `~~like this~~` to preserve historical pricing for the community. Database auto-trims old posts to stay lightweight.
* **Serverless Architecture:** 100% event-driven. Discord sends webhooks on commands. Google Cloud Scheduler wakes the bot up every minute to check Reddit.
+ * **Hardened Security:** Cryptographic interaction verification, non-root containers, and AI prompt guardrails to prevent injection.

## Deployment Guide (GCP & GitHub Actions)

### 1. Prerequisites
1. Create a [Discord Application](https://discord.com/developers/applications) and grab your **Bot Token**, **Public Key**, and **Application ID**.
2. Get a [Google Gemini API Key](https://aistudio.google.com/app/apikey).
3. Create a [Google Cloud Project](https://console.cloud.google.com/). Enable **Cloud Run**, **Firestore (Native Mode)**, and **Cloud Scheduler** APIs.
4. Create a GCP Service Account with `Editor` and `Cloud Run Admin` roles. Generate a JSON Key.

### 2. GitHub Secrets
Go to your GitHub Repository -> Settings -> Secrets and Variables -> Actions. Add the following repository secrets:
* `GCP_PROJECT_ID`: Your GCP Project ID (e.g., `my-cool-project-123`)
* `GCP_SA_KEY`: The entire JSON contents of the Service Account key you downloaded.
* `DISCORD_APP_ID`: From Discord Dev Portal
* `DISCORD_PUBLIC_KEY`: From Discord Dev Portal
* `DISCORD_BOT_TOKEN`: From Discord Dev Portal
* `GEMINI_API_KEY`: From Google AI Studio

### 3. Deploy
1. Push this code to the `main` branch. GitHub Actions will automatically build the Docker container and deploy it to a new Cloud Run service named `canadian-hardware-swap-bot`.
2. Once deployed, Cloud Run will give you a public URL (e.g., `https://canadian-hardware-swap-bot-xxxxxx.a.run.app`).

### 4. Link to Discord & Setup Cron
1. Go to the Discord Developer Portal -> Your App -> General Information. Paste your Cloud Run URL + `/interactions` into the **Interactions Endpoint URL** box (e.g., `https://canadian...run.app/interactions`). Discord will send a ping to verify it.
2. In the Google Cloud Console, go to **Cloud Scheduler**. Create a new job:
   * **Frequency:** `* * * * *` (Every minute)
   * **Target:** HTTP
   * **URL:** Your Cloud Run URL + `/cron/scrape` (e.g., `https://...run.app/cron/scrape`)
   * **HTTP Method:** GET

That's it! Invite the bot to your server and run `/setup`.

## Local Development (Ngrok)
To test the Discord interactions locally without deploying to GCP:
1. Create a `.env` file in the root of the project. **Never commit this file!** (It is added to `.gitignore` to prevent you from accidentally pushing it).
2. Fill your `.env` with:
   ```env
   DISCORD_PUBLIC_KEY=xxx
   DISCORD_BOT_TOKEN=xxx
   GEMINI_API_KEY=xxx
   GCP_PROJECT_ID=xxx
   ```
3. Run `ngrok http 8080`
4. Put the Ngrok HTTPS URL + `/interactions` into the Discord Dev Portal.
5. Run the Go server: `go run cmd/server/main.go`
