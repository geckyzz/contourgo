# Contour Go Discord Bot

[![Go Version](https://img.shields.io/github/go-mod/go-version/geckyzz/contourgo)](https://golang.org/)
[![License](https://img.shields.io/github/license/geckyzz/contourgo)](LICENSE)

A high-performance, concurrent Discord bot written in Go for monitoring comment updates on major
torrent trackers. Designed to be a robust, always-online replacement for legacy automation scripts.

## 🚀 Features

- **Multi-Site Monitoring**: Support for Nyaa/Sukebei, AnimeTosho (old/`.org` and new/clone/`.xyz`), nekoBT (torrents, comments, and user notifications), AniRena, TsukiHime, and Twitter/X (via Nitter RSS).
- **nekoBT User Notifications**: Poll user notifications from nekoBT:
  - **Interactive Button**: Message embeds are sent with a "Mark as Read" button. Clicking this calls the nekoBT API to mark that specific notification as read, and updates the Discord button component.
  - **Target channel override**: Support routing user notifications to dedicated/private channels to avoid public PII leakage.
- **Twitter/X Monitoring**: Poll public Twitter accounts via Nitter RSS feeds. Supports per-account:
  - **Embed services**: Rewrite tweet links to `fixupx.com`, `vxtwitter.com`, `fxtwitter.com`, `twittpr.com`, or custom domains for Discord previews.
  - **Custom message format**: Go template placeholders (`{{.Account}}`, `{{.Link}}`, etc.) to format text messages instead of default embeds.
  - **Target channel override**: Send announcements to a specific Discord channel per account.
  - **Per-account poll interval**: Independent check frequencies (`monitor.by`).
  - **Regex filtering**: Match or ignore tweets using case-insensitive regular expressions (`keywords` and `excludes`).
- **Concurrent Polling**: Utilizes Go routines to check active services in parallel.
- **Scraper Filtering**:
  - **Keywords**: Filter releases or comments by search strings (regex-compatible for Twitter).
  - **Uploaders**: Filter releases or comments by specific users (supported on Nyaa/Sukebei, nekoBT, AniRena).
  - **Groups**: Filter by fansub groups (supported on nekoBT, AniRena, TsukiHime).
  - **Media**: Filter by Series/Movie IDs (supported on nekoBT, TsukiHime).
    - nekoBT: Supports `tmdb:`, `tvdb:`, or internal IDs.
    - TsukiHime: Supports `mal:`, `anilist:`, `anidb:`, or internal IDs.
  - **Exclusions**: Skip items matching glob patterns (e.g. `*[REPACK]*`).
- **Discord Notifications**: Send Discord messages containing author details, formatted parent comment context for replies, and source links.
- **Dynamic Help Menu**: Interactive `/help` command displaying links to slash commands registered in the current bot session.
- **SQLite History**: Track comment states in SQLite to prevent duplicate announcements.
- **Migration Utilities**: Command `/import` to load comments from legacy json files.
- **Donation Role Manager**: Log contributions and manage server rewards:
  - Stackable subscription durations based on dollar value configurations (e.g. $9.99/mo).
  - Multi-tier server role assignments mapped by cumulative USD donation thresholds.
  - Check cycles to clear expired roles and send notification embeds.
  - Silent controls (globally or per command option) and transaction logs export in TSV format.

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

> [!TIP]
> **TOML Tip: Grouping Nested Settings**
>
> In TOML, instead of repeating prefixes using **dot notation** like this:
>
> ```toml
> [config]
> nyaa.sort = "comments"
> nyaa.order = "desc"
> ```
>
> You can **group** them cleanly under a nested sub-header:
>
> ```toml
> [config.nyaa]
> sort = "comments"
> order = "desc"
> ```
>
> Both formats work exactly the same way. Using grouped sub-headers makes it much easier to read and configure many options for a specific service!

### Global Config

| Key                                  | Type       | Description                                              | Required | Default              |
| :----------------------------------- | :--------- | :------------------------------------------------------- | :------- | :------------------- |
| **Discord Credentials & Setup**      |            |                                                          |          |                      |
| `discord.token`                      | String     | Your Discord Bot Token                                   | Yes      | —                    |
| `discord.server`                     | String/Int | Target server snowflake for instant command sync         | No       | —                    |
| `discord.announce.channel`           | String/Int | Discord channel snowflake for notifications              | Yes      | —                    |
| `discord.mentions_disable`           | Boolean    | Toggle to disable all mentions globally                  | No       | `false`              |
| **Mentions Mapping**                 |            |                                                          |          |                      |
| `discord.mentions`                   | Table/Map  | Map `@name` to `<@snowflake>` in message content         | No       | —                    |
| **Embed Layout & Styling**           |            |                                                          |          |                      |
| `discord.embed.author.url`           | String     | Global default static icon for the embed author          | No       | —                    |
| `discord.fields.comment_id`          | Boolean    | Global default to toggle rendering comment ID            | No       | `false`              |
| `discord.display.user_content_image` | Boolean    | Global default to toggle extracting user content images  | No       | `false`              |
| **Scraper Scheduling**               |            |                                                          |          |                      |
| `config.monitor.by`                  | String     | Check interval (e.g., `PT10M` or `10m`)                  | No       | `PT30M`              |
| `config.time.uniform`                | Boolean    | Align check schedules to interval boundaries             | No       | `false`              |
| **Service Integration APIs**         |            |                                                          |          |                      |
| `config.nyaa.proxy.url`              | String     | URL to your Nyaa/Sukebei API Proxy                       | Yes      | —                    |
| `config.nyaa.page.max`               | Integer    | Nyaa platform default page limit                         | No       | `0`                  |
| `config.nyaa.sort`                   | String     | Nyaa platform default sort method                        | No       | `"comments"`         |
| `config.nyaa.order`                  | String     | Nyaa platform default sort order                         | No       | `"desc"`             |
| `config.nekobt.api.key`              | String     | Your nekoBT SSID API key                                 | No       | —                    |
| `config.nekobt.page.max`             | Integer    | nekoBT platform default page limit                       | No       | `1`                  |
| `config.nekobt.sort`                 | String     | nekoBT platform default sort method                      | No       | `"date"`             |
| `config.anirena.api.key`             | String     | Your AniRena API key                                     | No       | —                    |
| `config.anirena.page.max`            | Integer    | AniRena platform default page limit                      | No       | `0`                  |
| `config.anirena.sort`                | String     | AniRena platform default sort method                     | No       | `"date"`             |
| `config.anirena.order`               | String     | AniRena platform default sort order                      | No       | `"desc"`             |
| `config.twitter.nitter_url`          | String     | Default base URL of Nitter instance to use               | No       | `https://nitter.net` |
| `config.twitter.embed_service`       | String     | Default global embed service domain/short-name to use    | No       | `x.com`              |
| `config.twitter.exclude_reposts`     | Boolean    | Default global setting to ignore retweet/repost items    | No       | `false`              |
| **Donation Management**              |            |                                                          |          |                      |
| `donation.currency`                  | String     | Currency code suffix (e.g. `USD`, `EUR`, `CAD`)          | No       | `"USD"`              |
| `donation.perk_multiplier`           | Float      | Cost per 1 month of role perks duration                  | No       | `9.99`               |
| `donation.max_stacks`                | Integer    | Max months perk duration stack limit                     | No       | `12`                 |
| `donation.notify_warn_days`          | Integer    | Days before expiry to send warning notification          | No       | `3`                  |
| `donation.silent.globally`           | Boolean    | Suppress all donator DMs globally                        | No       | `false`              |
| `donation.silent.on_warning`         | Boolean    | Suppress warning DM notifications                        | No       | `false`              |
| `donation.silent.on_expiry`          | Boolean    | Suppress expiry DM notifications                         | No       | `false`              |
| `donation.tiers`                     | Map/Table  | Multi-tier role mappings: `RoleID = MinAmountUSD`        | No       | —                    |
| **DM Notification Formats**          |            |                                                          |          |                      |
| `donation.format.add.title`          | String     | Go template for donation activation DM Embed Title       | No       | _(Default Text)_     |
| `donation.format.add.desc`           | String     | Go template for donation activation DM Embed Description | No       | _(Default Text)_     |
| `donation.format.renew.title`        | String     | Go template for renewal/extension DM Embed Title         | No       | _(Default Text)_     |
| `donation.format.renew.desc`         | String     | Go template for renewal/extension DM Embed Description   | No       | _(Default Text)_     |
| `donation.format.warn.title`         | String     | Go template for warning expiration DM Embed Title        | No       | _(Default Text)_     |
| `donation.format.warn.desc`          | String     | Go template for warning expiration DM Embed Description  | No       | _(Default Text)_     |
| `donation.format.expiry.title`       | String     | Go template for final expiry DM Embed Title              | No       | _(Default Text)_     |
| `donation.format.expiry.desc`        | String     | Go template for final expiry DM Embed Description        | No       | _(Default Text)_     |

#### DM Notification Placeholders

When customizing DM notification messages under `[donation.format.<action>]`, you can use the following Go template placeholder keys:

| Placeholder            | Description                                                         | Example Output                    |
| :--------------------- | :------------------------------------------------------------------ | :-------------------------------- |
| `{{.Username}}`        | Discord display username of the donator user                        | `geckyzz`                         |
| `{{.UserID}}`          | Discord numeric snowflake ID of the donator user                    | `123456789012345678`              |
| `{{.Amount}}`          | Formatted currency string added (e.g. `amount` + `USD`)             | `9.99 USD`                        |
| `{{.AmountValue}}`     | Raw float value of the current transaction amount                   | `9.99`                            |
| `{{.Cumulative}}`      | Cumulative formatted donation total (e.g. total cumulative + `USD`) | `19.98 USD`                       |
| `{{.CumulativeValue}}` | Raw float value of cumulative donation total                        | `19.98`                           |
| `{{.Duration}}`        | Formatted text representation of added time                         | `30 days`                         |
| `{{.Expiry}}`          | Full human-readable target date string                              | `January 2, 2026`                 |
| `{{.ExpiryUnix}}`      | Raw Unix epoch integer timestamp of the expiry date                 | `1767344400`                      |
| `{{.TimeLeft}}`        | Time left remaining (Only supported for warning DMs)                | `2 days, 23 hours`                |
| `{{.ServerName}}`      | Name of the Discord server/guild                                    | `Contour Go Server`               |
| `{{.Account}}`         | Payment account/gateway name (e.g. PayPal, Ko-Fi)                   | `paypal`                          |
| `{{.Note}}`            | Custom transaction note or memo                                     | `Hosting Support`                 |
| `{{.AddedDate}}`       | Formatted transaction creation date                                 | `January 1, 2026 at 12:00 AM MST` |
| `{{.AddedUnix}}`       | Raw Unix epoch timestamp of transaction creation                    | `1767225600`                      |
| `{{.RoleID}}`          | ID of the highest qualified server tier role assigned               | `132623942561261...`              |

### Monitor Blocks (`[monitors.<service>.<key>]`)

The `<key>` acts as a unique identifier for the monitor instance (used in logs, console outputs, and Discord announcement labels). For the **`twitter`** service, it also defaults to the Twitter username if `account` is omitted.

You can define multiple monitors per service.

| Option                                | Type          | Description                                                               | Required | Default       | Supported Services                                   |
| :------------------------------------ | :------------ | :------------------------------------------------------------------------ | :------- | :------------ | :--------------------------------------------------- |
| **Torrent Filters**                   |               |                                                                           |          |               |                                                      |
| `keywords`                            | List (String) | List of search strings (or regex for Twitter)                             | No       | `[]`          | All                                                  |
| `excludes`                            | List (String) | List of glob patterns (or regex for Twitter) to skip                      | No       | `[]`          | All                                                  |
| `uploaders`                           | List (String) | List of uploader usernames or IDs                                         | No       | `[]`          | Nyaa/Sukebei, nekoBT, AniRena                        |
| `groups`                              | List (String) | List of Group IDs or Group Slugs                                          | No       | `[]`          | nekoBT, AniRena, TsukiHime                           |
| `media`                               | List (String) | List of Media IDs                                                         | No       | `[]`          | nekoBT, TsukiHime                                    |
| **Query & Scraper Behavior**          |               |                                                                           |          |               |                                                      |
| `sort`                                | String        | Sorting method (see below)                                                | No       | _(Varies)_    | Nyaa/Sukebei, nekoBT, AniRena                        |
| `order`                               | String        | `asc` or `desc`                                                           | No       | `desc`        | Nyaa/Sukebei, AniRena                                |
| `page.max`                            | Integer       | Max pages to scan per check                                               | No       | _(Varies)_    | Nyaa/Sukebei, nekoBT, AniRena, Tsukihime, AnimeTosho |
| `monitor.by`                          | String        | Poll interval override for this monitor (e.g. `PT15M` or `15m`)           | No       | _(Inherit)_   | All                                                  |
| **Discord Customization & Overrides** |               |                                                                           |          |               |                                                      |
| `discord.mentions.disable`            | Boolean       | Toggle to disable all pings for this monitor                              | No       | `false`       | Nyaa/Sukebei, nekoBT, AniRena, Tsukihime, AnimeTosho |
| `discord.channel`                     | String/Int    | Discord channel override for announcements                                | No       | _(Inherit)_   | All                                                  |
| `discord.embed.author.url`            | String        | Static icon for the embed author (overrides global default)               | No       | _(Inherit)_   | Nyaa/Sukebei, nekoBT, AniRena, Tsukihime, AnimeTosho |
| `discord.fields.comment_id`           | Boolean       | Toggle rendering comment ID in embed (overrides global default)           | No       | _(Inherit)_   | Nyaa/Sukebei, nekoBT, AniRena, Tsukihime, AnimeTosho |
| `discord.display.user_content_image`  | Boolean       | Toggle extracting images from comment text to embeds (overrides global)   | No       | _(Inherit)_   | Nyaa/Sukebei, nekoBT, AniRena, Tsukihime, AnimeTosho |
| **Twitter/X Settings**                |               |                                                                           |          |               |                                                      |
| `account`                             | String        | Twitter username (without @); defaults to monitor key                     | No       | _(Keyname)_   | twitter                                              |
| `exclude_reposts`                     | Boolean       | Ignore retweet/repost items (starts with "RT by @")                       | No       | `false`       | twitter                                              |
| `nitter_url`                          | String        | Override global Nitter base URL for this monitor                          | No       | _(Inherit)_   | twitter                                              |
| `embed_service`                       | String        | Rewrite tweet links for Discord preview (see embed services table below). | No       | `x.com`       | twitter                                              |
| `custom_format`                       | String        | Go template string for content override (see placeholders table below).   | No       | _(Tweet URL)_ | twitter                                              |

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

#### NekoBT Notifications Monitoring

**NekoBT** (`nekobt`) supports monitoring user notifications.

If you define a monitor block with the keyname `notification` or `notifications` (for example, `[monitors.nekobt.notification]`), it will poll user notifications instead of torrent comments.

- **Interactive Buttons**: The Discord announcement sent for each notification includes a "Mark as Read" button.
- **Access Control**: Only authorized administrators or moderators (based on the bot's permission rules) are allowed to click the button and mark the notification as read.
- **PII Leakage Prevention**: It is highly recommended to override the target channel (using `discord.channel = "CHANNEL_ID"`) inside the notification monitor block to route these private alerts to a restricted/private channel.
- **Custom Formatting & Mentions**: You can configure a `custom_format` template string (e.g. `custom_format = "<@1234567890> — {{.Message}}"`) to override the plain-text message body. This allows you to tag specific users or roles. The only supported template variable is `{{.Message}}` which resolves to the notification content.

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
- `/monitors` - Manage and inspect configured monitors:
  - `list` - Lists all configured monitors, showing status (🟢 active or ⏸️ paused) and filters.
  - `pause <service> <key>` - Pauses a specific monitor (skips checks and suppresses announcements).
  - `resume <service> <key>` - Resumes a paused monitor.
  - `force <service> <key>` - Force check a specific monitor immediately.
- `/reload` - Reloads the configuration file (hot-reload `config.toml`) or triggers a manual monitor
  check cycle. Options: `target` (`monitors` or `config`).
- `/ping` - Checks bot heartbeat latency.
- `/logs` - View recent log entries (last 20-100 lines).
- `/latest` - Show recently discovered torrents.
- `/import` - Migrate legacy JSON data (supports key-based decryption).
- `/test` - Debug a search query against any service.
- `/donation` - Manage server donation logs and roles:
  - `add <user> <amount> [account] [note] [end_date] [silent]` - Log a donation, add stacked duration (multiplier $9.99), sync multi-tier roles, and send DM (silence-able).
  - `status <user>` - View a user's active/expired state, remaining duration, and total contributions.
  - `list` - List all active donators, total contribution, and active time-left.
  - `export [user]` - Export raw donation logs in TSV format as a download attachment.
  - `history <user>` - View donation history logs for a user as an embed.
  - `check` - Force an expiration cycle evaluation immediately.
  - `manage delete <user>` - Delete a donator record and all their logs, and strip all tier roles.
  - `manage edit <user> [total] [expiry]` - Override a donator's cumulative total and/or expiry date, and re-sync roles.
  - `manage delete_log <log_id>` - Delete a single donation log entry by its numeric ID (use `export` to find IDs).
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
