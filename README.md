# Plex Presence

A lightweight Windows system tray application that displays your Plex playback activity as Discord Rich Presence.

## Features

- Shows currently playing movie or TV episode in Discord
- Displays elapsed time and progress bar
- Updates paused state in real time
- Clears presence when playback stops
- Filters to your local machine only — other household members' sessions are ignored
- Launch at Windows startup toggle in the tray menu
- Single compiled executable, no runtime dependencies

## Requirements

- Windows
- Discord desktop client
- Plex Media Server (local network)
- A Discord application with Rich Presence enabled

## Setup

### 1. Create a Discord Application

1. Go to [https://discord.com/developers/applications](https://discord.com/developers/applications)
2. Click **New Application** and give it a name (this appears as "Watching **[name]**" in Discord)
3. Copy the **Application ID** from the General Information page
4. Go to **Rich Presence → Art Assets** and upload a image named `plex` for the large image

### 2. Get Your Plex Token

1. Open Plex Web in your browser
2. Open browser devtools (F12) and go to the **Network** tab
3. Click anything in your Plex library to trigger a request
4. Find any request to your Plex server and check the request headers for `X-Plex-Token`

### 3. Configure the App

Copy `config.json.example` to `config.json` and fill in your values:

```json
{
    "plexHost": "http://your-server-hostname:32400",
    "plexToken": "your-plex-token-here",
    "discordClientID": "your-discord-app-id-here"
}
```

`config.json` is gitignored and should never be committed.

### 4. Build