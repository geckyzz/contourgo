# Contour Go Discord Bot

[![Go Version](https://img.shields.io/github/go-mod/go-version/geckyzz/contourgo)](https://golang.org/)
[![License](https://img.shields.io/github/license/geckyzz/contourgo)](LICENSE)

A high-performance, concurrent Discord bot written in Go for monitoring comment updates on major
torrent trackers. Designed to be a robust, always-online replacement for legacy automation scripts.

## 🚀 Features

- **Multi-Site Monitoring**: Support for **nyaa.si**, **sukebei.nyaa.si**, **AnimeTosho**
  (beta/old/`.org` and new/clone/`.xyz`), **nekoBT**, and **AniRena**.
- **Parallel Architecture**: Utilizes Go routines to check all active services simultaneously, maximizing
  throughput and minimizing check cycle duration.
- **Smart Filtering**:
  - **Keywords**: Monitor specific search terms.
  - **Uploaders**: (Nyaa, Sukebei, nekoBT, AniRena) Monitor all releases from specific users.
  - **Groups**: (nekoBT, AniRena) Monitor specific fansub groups.
  - **Media**: (nekoBT) Monitor specific series or movies by ID (supports nekoBT, TMDB, and TVDB IDs).
  - **Exclusions**: Skip torrents matching glob patterns (e.g., `*[REPACK]*`).
- **Optimized Scraping**:
  - **Server-Side Sorting**: Requests results sorted by comments to enable early-exit optimizations.
  - **Early Break**: Immediately stops searching once torrents with zero comments are reached.
  - **Smart Pagination**: Native layout parsing to detect the next page, avoiding unnecessary empty
    API calls.
- **Rich Notifications**: Beautiful Discord embeds with author info, truncated parent comment context
  for replies, and direct links.
- **Adaptive Help**: An interactive `/help` menu that dynamically links to actual slash commands
  registered in your session.
- **Persistent History**: Backed by SQLite to ensure no comment is announced twice.
- **Migration Support**: Built-in `/import` command to seamlessly migrate history from [nyaa_comments][nc].

[nc]: https://github.com/geckyzz/nyaa_comments

---

## 🛠️ Installation

### 1. Build from Source

Ensure you have Go installed (1.20+ recommended).

```bash
git clone https://github.com/geckyzz/contourgo.git
cd contourgo
go build -o contourgo main.go
```

### 2. Configure Settings

Copy the example configuration and fill in your details:

```bash
cp config.toml.example config.toml
```

> [!IMPORTANT]
> **Nyaa/Sukebei Monitoring**: You **must** have a running instance of
> [nyaa-api-worker](https://github.com/geckyzz/nyaa-api-worker). The bot offloads complex
> HTML parsing to this proxy to receive clean RESTful JSON.

### 3. Run the Bot

```bash
./contourgo
```

For initial database seeding without spamming Discord:

```bash
./contourgo --dump-comments
```

---

## ⚙️ Configuration Guide

### Global Config

| Key                        | Description                                      | Default             |
| :------------------------- | :----------------------------------------------- | :------------------ |
| `discord.token`            | Your Discord Bot Token                           | (Required)          |
| `discord.server`           | Target server snowflake for instant command sync | (Optional)          |
| `discord.announce.channel` | Discord channel snowflake for notifications      | (Required)          |
| `config.monitor.by`        | Check interval (e.g., `PT10M` or `10m`)          | `PT30M`             |
| `config.nyaa.proxy.url`    | URL to your Nyaa API Proxy                       | (Required for Nyaa) |
| `config.nekobt.api.key`    | Your nekoBT SSID API key                         | (Optional)          |
| `config.anirena.api.key`   | Your AniRena API key                             | (Optional)          |

### Monitor Blocks (`[monitor.<service>.<key>]`)

You can define multiple monitors per service.

| Option      | Description                                        | Supported Services             |
| :---------- | :------------------------------------------------- | :----------------------------- |
| `keywords`  | List of search strings                             | All                            |
| `excludes`  | List of glob patterns to skip                      | All                            |
| `uploaders` | List of uploader usernames or IDs                  | Nyaa, Sukebei, nekoBT, AniRena |
| `groups`    | List of Group IDs or Group Slugs                   | nekoBT, AniRena                |
| `media`     | List of Media IDs (`s123`, `tmdb:123`, `tvdb:456`) | nekoBT                         |
| `sort`      | Sorting method (see below)                         | Nyaa, Sukebei, nekoBT, AniRena |
| `order`     | `asc` or `desc`                                    | Nyaa, Sukebei, AniRena         |
| `page.max`  | Max pages to scan per check                        | All                            |

**Supported Services**: `nyaa`, `sukebei`, `animetosho_old`, `animetosho_new`, `nekoBT`, `anirena`.

#### Sorting Options:

- **Nyaa/Sukebei**: **`id`** (date), `comments`, `size`, `seeders`, `leechers`, `downloads`.
- **nekoBT**: `best`, **`latest`**, `oldest`, `rss`, `seeders`, `seeders_asc`, `leechers`, `leechers_asc`,
  `downloads`, `downloads_asc`, `comments`, `comments_asc`, `filesize`, `filesize_asc`.
- **AniRena**: `sort`: **`date`**, `size`, `seeders`, `leechers`, `completed`, `title`.

---

## 💬 Slash Commands

- `/status` - Displays bot health, active monitors, system/memory diagnostics, and DB stats.
- `/monitors` - Lists all currently active monitor definitions.
- `/reload` - Reloads the configuration file (hot-reload `config.toml`) or triggers a manual monitor
   check cycle. Options: `target` (`monitors` or `config`).
- `/ping` - Checks bot heartbeat latency.
- `/logs` - View recent log entries (last 20-100 lines).
- `/latest` - Show recently discovered torrents.
- `/import` - Migrate legacy JSON data (supports key-based decryption).
- `/test` - Debug a search query against any service.
- `/help` - Interactive, linked command menu.

---

## 📦 Versioning & Releases

To prepare and trigger a new SemVer release:

1. Run the version-bumping script:
   ```bash
   ./bump-version.sh [major|minor|patch]
   ```
   This automatically increments the version, updates the source code versions in `bot.go`, and creates
   a local git commit and tag.

2. Push to GitHub to trigger the CI build and release pipeline:
   ```bash
   git push && git push --tags
   ```
   The GitHub Actions CI pipeline will automatically build optimized binaries for **Linux (amd64)**
   and **Windows (amd64)**, and package them as assets in a new GitHub Release.

---

## 📜 License

This project is licensed under the AGPL License - see the [LICENSE](LICENSE) file for details.
