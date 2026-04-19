# docs/images/

Screenshots referenced from the root `README.md`. If any are missing, the inline image will render as a broken-link box but the prose walkthrough next to it is still complete and sufficient.

## Expected files

| Filename | What it shows | Captured from |
|---|---|---|
| `grafana-cloud-collector-sidebar.png` | Grafana Cloud left sidebar with **Connections → Collector → Collector Setup** expanded and highlighted | Any Grafana Cloud stack sidebar |
| `grafana-cloud-platform-token.png` | Wide view: Linux/macOS/Windows/Kubernetes tabs with **macOS** selected, **Install Alloy** platform/arch/method dropdowns, and the **Use an API token** section showing a generated `glc_...` token | Connections → Collector → Collector Setup, after picking macOS |
| `grafana-cloud-api-token-detail.png` | Close-up of the **Use an API token** form: **Create a new token** tab, Token name input, Expiration date (`No expiry`), Scopes (`set:alloy-data-write`), and the **Create token** button | Same page, cropped tight on the token creation form |
| `grafana-cloud-install-commands.png` | The three install command blocks (Download binary, Set up the Alloy configuration, Run Alloy) with `GCLOUD_*` env vars populated from your stack | Same page, bottom section after enabling Remote Configuration |

## How to capture

1. Log into your Grafana Cloud stack.
2. Navigate to **Connections → Collector → Collector Setup**.
3. Select **macOS**, **Arm64**, **Binary**.
4. Create an API token (scope `set:alloy-data-write`, no expiry is fine for a laptop).
5. Toggle **Enable Remote Configuration** on.
6. Screenshot each of the three regions above. `Cmd+Shift+4`, drag, drop here.

**Redact before committing:** the `GCLOUD_RW_API_KEY` (starts with `glc_`), instance IDs, and hosted URLs in the command blocks are tenant-specific credentials. If you screenshot with real values visible, blur them before committing.
