# Contour Go Discord Bot

[![Go Version](https://img.shields.io/github/go-mod/go-version/geckyzz/contourgo)](https://golang.org/)
[![License](https://img.shields.io/github/license/geckyzz/contourgo)](LICENSE)

A high-performance, concurrent Discord bot written in Go for monitoring comment updates on major
torrent trackers. Designed to be a robust, always-online replacement for legacy automation scripts.

## 🚀 Features

- **Multi-Site Monitoring**: Support for **Nyaa/Sukebei**, **AnimeTosho**
  (old/`.org` and new/clone/`.xyz`), **nekoBT**, **AniRena**, **TsukiHime**, and
  **Twitter/X** (via Nitter RSS).
- **Twitter/X Monitoring**: Poll any public Twitter account via a [Nitter](https://github.com/zedeus/nitter)
  RSS feed. Supports per-account:
  - **Embed services**: Rewrite tweet links to `fixupx.com`, `vxtwitter.com`, `fxtwitter.com`,
    `twittpr.com`, or any custom domain for better Discord preview support.
  - **Custom message format**: Use Go template placeholders (`{{.Account}}`, `{{.Link}}`, etc.)
    to send a plain content message instead of an embed.
  - **Target channel override**: Route announcements to a specific channel per account.
  - **Per-account poll interval**: Independent `monitor.by` for each account.
  - **Regex filtering**: Optional case-insensitive regular expressions matching for `keywords` and `excludes`.
- **Parallel Architecture**: Utilizes Go routines to check all active services simultaneously, maximizing
  throughput and minimizing check cycle duration.
- **Smart Filtering**:
  - **Keywords**: Monitor specific search terms across all services (supports regex on Twitter).
  - **Uploaders**: (Nyaa/Sukebei, nekoBT, AniRena) Monitor all releases or comments from specific users.
  - **Groups**: (nekoBT, AniRena, TsukiHime) Monitor specific fansub groups.
  - **Media**: (nekoBT, TsukiHime) Monitor specific series or movies by ID.
    - **nekoBT**: Supports `tmdb:`, `tvdb:`, or media slug.
    - **TsukiHime**: Supports `mal:`, `anilist:`, `anidb:`, or internal ID (e.g., `79`).
  - **Exclusions**: Skip torrents matching glob patterns (e.g., `*[REPACK]*`).
- **Rich Notifications**: Beautiful Discord embeds with author info, truncated parent comment context
  for replies, and direct links.
- **Adaptive Help**: An interactive `/help` menu that dynamically links to actual slash commands
  registered in your session.
- **Persistent History**: Backed by SQLite to ensure no comment is announced twice.
- **Migration Support**: Built-in `/import` command to seamlessly migrate history from
  [nyaa_comments][nc].

[nc]: https://github.com/geckyzz/nyaa_comments

---

## 🛠️ Installation

### 1. Download (Recommended)

The easiest way to run the bot is to download the latest pre-compiled binary for your operating system
(Linux or Windows) from the [**GitHub Releases**](https://github.com/geckyzz/contourgo/releases)
page. Just downlaod it and you're good to go!

### 1b. Build from Source (For Masochists)

If you enjoy compiling things yourself, ensure you have Go installed (1.20+ recommended).

```bash
git clone https://github.com/geckyzz/contourgo.git
cd ./contourgo
go build -o contourgo main.go
```

### 2. Configure Settings

Copy the example configuration and fill in your details:

```bash
cp config.toml.example config.toml
```

> [!IMPORTANT]
> **Nyaa/Sukebei Monitoring**: You **must** have a running instance of
> [nyaa-api-worker](https://github.com/geckyzz/nyaa-api-worker). Both services offload complex
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

| Key                                  | Description                                             | Required | Default              |
| :----------------------------------- | :------------------------------------------------------ | :------- | :------------------- |
| **Discord Credentials & Setup**      |                                                         |          |                      |
| `discord.token`                      | Your Discord Bot Token                                  | Yes      | —                    |
| `discord.server`                     | Target server snowflake for instant command sync        | No       | —                    |
| `discord.announce.channel`           | Discord channel snowflake for notifications             | Yes      | —                    |
| **Mentions Mapping**                 |                                                         |          |                      |
| `discord.mentions`                   | Map `@name` to `<@snowflake>` in message content        | No       | —                    |
| **Embed Layout & Styling**           |                                                         |          |                      |
| `discord.embed.author.url`           | Global default static icon for the embed author         | No       | —                    |
| `discord.fields.comment_id`          | Global default to toggle rendering comment ID           | No       | `false`              |
| `discord.display.user_content_image` | Global default to toggle extracting user content images | No       | `false`              |
| **Scraper Scheduling**               |                                                         |          |                      |
| `config.monitor.by`                  | Check interval (e.g., `PT10M` or `10m`)                 | No       | `PT30M`              |
| `config.time.uniform`                | Align check schedules to interval boundaries            | No       | `false`              |
| **Service Integration APIs**         |                                                         |          |                      |
| `config.nyaa.proxy.url`              | URL to your Nyaa/Sukebei API Proxy                      | Yes      | —                    |
| `config.nekobt.api.key`              | Your nekoBT SSID API key                                | No       | —                    |
| `config.anirena.api.key`             | Your AniRena API key                                    | No       | —                    |
| `config.twitter.nitter_url`          | Default base URL of Nitter instance to use              | No       | `https://nitter.net` |
| `config.twitter.embed_service`       | Default global embed service domain/short-name to use   | No       | `x.com`              |

### Monitor Blocks (`[monitors.<service>.<key>]`)

You can define multiple monitors per service.

| Option                                | Description                                                               | Required | Default       | Supported Services            |
| :------------------------------------ | :------------------------------------------------------------------------ | :------- | :------------ | :---------------------------- |
| **Torrent Filters**                   |                                                                           |          |               |                               |
| `keywords`                            | List of search strings (regex-compatible for Twitter)                     | No       | `[]`          | All                           |
| `excludes`                            | List of glob patterns (regex-compatible for Twitter) to skip              | No       | `[]`          | All                           |
| `uploaders`                           | List of uploader usernames or IDs                                         | No       | `[]`          | Nyaa/Sukebei, nekoBT, AniRena |
| `groups`                              | List of Group IDs or Group Slugs                                          | No       | `[]`          | nekoBT, AniRena, TsukiHime    |
| `media`                               | List of Media IDs                                                         | No       | `[]`          | nekoBT, TsukiHime             |
| **Query & Scraper Behavior**          |                                                                           |          |               |                               |
| `sort`                                | Sorting method (see below)                                                | No       | _(Varies)_    | Nyaa/Sukebei, nekoBT, AniRena |
| `order`                               | `asc` or `desc`                                                           | No       | `desc`        | Nyaa/Sukebei, AniRena         |
| `page.max`                            | Max pages to scan per check                                               | No       | `5`           | All                           |
| **Discord Customization & Overrides** |                                                                           |          |               |                               |
| `discord.mentions.disable`            | Toggle to disable all pings for this monitor                              | No       | `false`       | All                           |
| `discord.channel`                     | Discord channel override for announcements                                | No       | _(Inherit)_   | All                           |
| `discord.embed.author.url`            | Static icon for the embed author (overrides global default)               | No       | _(Inherit)_   | All                           |
| `discord.fields.comment_id`           | Toggle rendering comment ID in embed (overrides global default)           | No       | _(Inherit)_   | All                           |
| `discord.display.user_content_image`  | Toggle extracting images from comment text to embeds (overrides global)   | No       | _(Inherit)_   | All                           |
| **Twitter/X Settings**                |                                                                           |          |               |                               |
| `account`                             | Twitter username (without @); defaults to monitor key                     | No       | _(Keyname)_   | twitter                       |
| `nitter_url`                          | Override global Nitter base URL for this monitor                          | No       | _(Inherit)_   | twitter                       |
| `embed_service`                       | Rewrite tweet links for Discord preview (see embed services table below). | No       | `x.com`       | twitter                       |
| `custom_format`                       | Go template string for content override (see placeholders table below).   | No       | _(Tweet URL)_ | twitter                       |

**Supported Services**: `nyaa`, `sukebei`, `animetosho_old`, `animetosho_new`, `nekobt`, `anirena`,
`tsukihime`, `twitter`.

#### Media ID Formats

- **nekoBT**: `s123` (internal ID), `tmdb:123`, `tvdb:456`.
- **TsukiHime**: `79` (internal ID), `mal:59970`, `anilist:196187`, `anidb:19479`.

#### Twitter Custom Formatting Keys

When configuring `custom_format`, you can use the following Go template placeholder keys:

| Placeholder         | Description                                                                         | Example Output                                                 |
| :------------------ | :---------------------------------------------------------------------------------- | :------------------------------------------------------------- |
| `{{.Account}}`      | The Twitter/X username (without `@`)                                                | `mofusand_anime`                                               |
| `{{.DisplayName}}`  | The display name from the Nitter RSS channel                                        | `アニメ『mofusand』公式`                                       |
| `{{.TweetID}}`      | The parsed numeric tweet ID (falls back to RSS GUID)                                | `2069557366060716144`                                          |
| `{{.Link}}`         | The rewritten tweet URL (uses the `embed_service` domain, else defaults to `x.com`) | `https://fixupx.com/mofusand_anime/status/2069557366060716144` |
| `{{.OriginalLink}}` | The original `x.com` status link                                                    | `https://x.com/mofusand_anime/status/2069557366060716144`      |
| `{{.Title}}`        | The RSS item title (containing the tweet text excerpt)                              | `🐾今日の #mofusand🐾 にゃー！！ #ちびにゃん`                  |
| `{{.PublishedAt}}`  | The Unix timestamp of when the tweet was published (int64)                          | `1781214364`                                                   |

#### Supported Twitter Embed Services

When configuring `embed_service`, you can use standard short names, custom domains, or leave it empty for defaults:

| Embed Service / Domain                | Discord Preview Behavior                  | Rewritten Link Example                                             |
| :------------------------------------ | :---------------------------------------- | :----------------------------------------------------------------- |
| `fixupx` (or `fixupx.com`)            | Optimized video and image previews.       | `https://fixupx.com/mofusand_anime/status/2069557366060716144`     |
| `vxtwitter` (or `vxtwitter.com`)      | Enhances video playback embeds.           | `https://vxtwitter.com/mofusand_anime/status/2069557366060716144`  |
| `fxtwitter` (or `fxtwitter.com`)      | Stable video & image previews.            | `https://fxtwitter.com/mofusand_anime/status/2069557366060716144`  |
| `twittpr` (or `twittpr.com`)          | Rewrites to use `twittpr.com`.            | `https://twittpr.com/mofusand_anime/status/2069557366060716144`    |
| `fixvx` (or `fixvx.com`)              | Rewrites to use `fixvx.com`.              | `https://fixvx.com/mofusand_anime/status/2069557366060716144`      |
| Custom Domain (e.g. `yourdomain.com`) | Rewrites to the provided domain directly. | `https://yourdomain.com/mofusand_anime/status/2069557366060716144` |
| `""` (Empty / Not Set)                | Fallback to original `x.com` format.      | `https://x.com/mofusand_anime/status/2069557366060716144`          |

#### Sorting Options

- **Nyaa/Sukebei**: **`id`** (date), `comments`, `size`, `seeders`, `leechers`, `downloads`.
- **nekoBT**: `best`, **`latest`**, `oldest`, `rss`, `seeders`, `seeders_asc`, `leechers`, `leechers_asc`,
  `downloads`, `downloads_asc`, `comments`, `comments_asc`, `filesize`, `filesize_asc`.
- **AniRena**: `sort`: **`date`**, `size`, `seeders`, `leechers`, `completed`, `title`.

#### Feedback Monitoring

**AnimeTosho** (`animetosho_old`/`animetosho_new`) and **TsukiHime** (`tsukihime`) support monitoring
feedback pages.

If you define a monitor block with the keyname `feedback` (for example, `[monitors.tsukihime.feedback]`),
it will monitor general feedback comments instead of specific torrent comments.

If you provide a keyword on feedback monitor, it will be matched against the comment message content
(case-insensitive).

#### Mention Mapping (`discord.mentions`)

The bot can be configured to map specific strings found in comments (e.g., `@geckyzz`) to Discord user snowflakes. **Mapping is case-insensitive.**

- **Announcement behavior**: If a mapped name is found in a comment (e.g., `@Geckyzz` or `@geckyzz`), the bot will include the corresponding Discord mention in the message content (the text above the embed). This ensures the user is pinged. The original text inside the embed description remains unchanged.
- **Monitor-specific toggle**: You can use `discord.mentions.disable = true` in a monitor block to completely suppress pings for that specific monitor.
- **Verification**: The `/test` command will report any detected mentions in plain text to verify your configuration.

**Configuration Example**:

```toml
[discord]
mentions = { "geckyzz" = 123456789012345678, "cicak" = "876543210987654321" }
```

---

### ⏱️ Uniform Scheduling Alignment (`config.time.uniform`)

When `config.time.uniform` is set to `true`, the bot aligns all check times to the natural boundaries of each monitor's configured interval. This ensures tasks run "on time" relative to the clock (e.g., exactly at `:00`, `:15`, `:30`, `:45`), even if the bot is booted at an arbitrary time.

#### How it works:

1. **Immediate Startup Check**: The bot performs its initial checks immediately on startup without any delay.
2. **Dynamic Boundary Alignment**: Instead of sleeping, the bot offsets the `lastCheck` timestamp of each monitor backward to the closest natural divisor boundary of its interval duration.
3. **No Drift / Shrinkage**: Future checks will trigger precisely on-time relative to the clock according to the interval.

#### Example Scenario:

If the bot is booted at **`00:05`**:

- **Global Interval (`15m`)**:
  - Runs immediately on boot at **`00:05`**.
  - Aligns `lastCheck` to **`00:00`** (the previous 15-minute boundary).
  - Subsequent checks run at exactly **`00:15`**, **`00:30`**, **`00:45`**, **`01:00`** etc.
- **Monitor Override (`30m`)**:
  - Runs immediately on boot at **`00:05`**.
  - Aligns `lastCheck` to **`00:00`** (the previous 30-minute boundary).
  - Subsequent checks run at exactly **`00:30`**, **`01:00`**, **`01:30`** etc.

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
