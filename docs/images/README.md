# docs/images/

Screenshots referenced from the root `README.md`. If any are missing, the inline image will render as a broken-link box but the prose walkthrough next to it is still complete and sufficient.

## Expected files

| Filename | What it shows | Captured from |
|---|---|---|
| `grafana-cloud-collector-sidebar.png` | Grafana Cloud left sidebar with **Connections → Collector → Collector Setup** expanded and highlighted | Any Grafana Cloud stack sidebar |
| `grafana-cloud-platform-token.png` | Wide view: Linux/macOS/Windows/Kubernetes tabs with **macOS** selected, **Install Alloy** platform/arch/method dropdowns, and the **Use an API token** section showing a generated `glc_...` token | Connections → Collector → Collector Setup, after picking macOS |
| `grafana-cloud-api-token-detail.png` | Close-up of the **Use an API token** form: **Create a new token** tab, Token name input, Expiration date (`No expiry`), Scopes (`set:alloy-data-write`), and the **Create token** button | Same page, cropped tight on the token creation form |
| `grafana-cloud-install-commands.png` | The three install command blocks (Download binary, Set up the Alloy configuration, Run Alloy) with `GCLOUD_*` env vars populated from your stack | Same page, bottom section after enabling Remote Configuration |
| `dashboard-overview.png` | Close-up of the dashboard's **Overview** row — 6 headline stats (Commits / PRs / Lines+ / Lines− / Tokens / Cost), 4 derived stats (Cache Hit Rate, Tool Accept Rate, Cost per Session, Tool Decisions), and the Active Time timeseries | A live render of `grafana.json` after at least one `spawn-claude run` session has flowed data |
| `dashboard-full.png` | Full vertical render of all five rows: Overview, Cost & Tokens, Activity & Productivity, Leaderboards, Request & Tool Activity (via Loki) | Same source, full-page screenshot |
| `dashboard-loki.png` | Close-up of the **Request & Tool Activity (via Loki)** row — three rate timeseries (api_request, user_prompt, tool_decision/result) plus the Recent Events log stream showing structured Claude Code events | Same source, scrolled to the Loki row |

## How to capture (Grafana Cloud onboarding shots)

1. Log into your Grafana Cloud stack.
2. Navigate to **Connections → Collector → Collector Setup**.
3. Select **macOS**, **Arm64**, **Binary**.
4. Create an API token (scope `set:alloy-data-write`, no expiry is fine for a laptop).
5. Toggle **Enable Remote Configuration** on.
6. Screenshot each of the three regions above. `Cmd+Shift+4`, drag, drop here.

**Redact before committing:** the `GCLOUD_RW_API_KEY` (starts with `glc_`), instance IDs, and hosted URLs in the command blocks are tenant-specific credentials. If you screenshot with real values visible, blur them before committing.

## How to capture (dashboard shots)

The three `dashboard-*.png` files were captured from a public Grafana snapshot URL using headless Chromium via Playwright. The snapshot URL embeds a fixed time range so the screenshots are reproducible. To regenerate:

1. In Grafana, open the dashboard, hit **Share → Snapshot → Local Snapshot** (or **Publish to snapshots.raintank.io** for an external link), copy the URL.
2. Drive Chromium with the URL — Grafana lazy-loads offscreen rows, so a naive single-shot only captures the first viewport. Either use `playwright screenshot --full-page` and accept that bottom rows may be blank, or write a small Python/Node script that scrolls progressively before screenshotting (see `/tmp/grafana-shot.py` in the session that produced these images for a working pattern).
3. Append `&kiosk` to the snapshot URL to hide Grafana's chrome.

The dashboard shots have no credentials in them — Grafana snapshots strip auth tokens and you control the time range. Safe to commit as-is.
