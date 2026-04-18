# Using spawn-claude with Grafana Cloud's onboarding flow

If you onboarded to Grafana Cloud via its **Install Alloy on macOS** UI and ran the `install-macos-binary.sh` one-liner, then later installed `spawn-claude` to manage the LaunchDaemon — this doc is for you. You will hit a specific authentication failure on first boot, and the cause is not obvious.

Everyone else (who configures via `spawn-claude collector configure <preset>`) can ignore this doc.

## TL;DR

```bash
# From a shell where GCLOUD_RW_API_KEY is already exported (e.g. via .zshrc):
sudo /usr/libexec/PlistBuddy \
  -c "Delete :EnvironmentVariables:GCLOUD_RW_API_KEY" \
  /Library/LaunchDaemons/com.grafana.alloy.plist 2>/dev/null
sudo /usr/libexec/PlistBuddy \
  -c "Add :EnvironmentVariables:GCLOUD_RW_API_KEY string ${GCLOUD_RW_API_KEY}" \
  /Library/LaunchDaemons/com.grafana.alloy.plist
sudo plutil -lint /Library/LaunchDaemons/com.grafana.alloy.plist
sudo launchctl bootout system/com.grafana.alloy
sudo launchctl bootstrap system /Library/LaunchDaemons/com.grafana.alloy.plist
```

Read on if you want to understand why.

---

## The config Grafana gave you

`install-macos-binary.sh` downloads a config template, fills in your stack numeric IDs, and `sudo mv`s it into `/etc/alloy/config.alloy`. The two auth blocks look like this:

```hcl
prometheus.remote_write "metrics_service" {
  endpoint {
    url = "https://prometheus-prod-XX-prod-<region>.grafana.net/api/prom/push"
    basic_auth {
      username = "<your-instance-id>"
      password = sys.env("GCLOUD_RW_API_KEY")   // ← reads env var at eval time
    }
  }
}

loki.write "grafana_cloud_loki" {
  endpoint {
    url = "https://logs-prod-XXX.grafana.net/loki/api/v1/push"
    basic_auth {
      username = "<your-instance-id>"
      password = sys.env("GCLOUD_RW_API_KEY")
    }
  }
}
```

`sys.env("NAME")` is an Alloy function that reads the environment variable `NAME` **from whatever process is running Alloy**, at config-eval time. Grafana's onboarding expects you to run Alloy like this in a terminal:

```bash
GCLOUD_RW_API_KEY="glc_..." ~/alloy-darwin-arm64 run ... /etc/alloy/config.alloy
```

The `alloy` process inherits `GCLOUD_RW_API_KEY` from your interactive shell; `sys.env()` reads it; the HTTP basic-auth password is correct; writes to Grafana Cloud succeed.

## Why it breaks under a LaunchDaemon

`spawn-claude collector install` stops using hand-run and instead loads Alloy as a LaunchDaemon at `/Library/LaunchDaemons/com.grafana.alloy.plist`. LaunchDaemons are started by `launchd` (pid 1), not by your shell, and they inherit **only** the env vars explicitly declared in the plist's `<key>EnvironmentVariables</key>` dict — plus launchd's own minimal defaults like `HOME=/var/root` and `USER=root`.

None of your shell profile runs. `.zshrc`, `.zprofile`, `.zshenv`, `/etc/zshenv`, `~/.profile`, `direnv`, `asdf`, `mise` — all of it is invisible to the daemon. This is by design: LaunchDaemons run before login and need to be deterministic.

The plist that `spawn-claude` installs declares only `PATH`:

```xml
<key>EnvironmentVariables</key>
<dict>
    <key>PATH</key>
    <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
</dict>
```

So when Alloy evaluates your config, `sys.env("GCLOUD_RW_API_KEY")` returns `""`. The HTTP request Alloy sends to Grafana has an empty-string basic-auth password. Grafana returns `401 Unauthorized: invalid token`. You'll see the auth failures immediately in `/var/log/alloy/stderr.log`:

```
level=error msg="non-recoverable error" component_path=/ component_id=prometheus.remote_write.metrics_service ... status 401 Unauthorized
level=error msg="final error sending batch, no retries left, dropping data" component_id=loki.write.grafana_cloud_loki ... status=401
```

OTLP reception (Claude → Alloy, on 127.0.0.1:4317/4318) still works fine — that's in-process, no Grafana credentials required. What's broken is the Alloy → Grafana Cloud basic-auth leg.

## The fix

Inject `GCLOUD_RW_API_KEY` directly into the installed plist's `EnvironmentVariables` dict, so launchd sets it in the daemon's process env:

```bash
# Assumes GCLOUD_RW_API_KEY is exported in your current shell.
# Check with:  echo ${#GCLOUD_RW_API_KEY}   (should be ~100, not 0)
# If it's 0, your shell doesn't have it exported — this is common if the only
# time you set it was inline on a hand-run command (e.g. `GCLOUD_RW_API_KEY=...
# alloy run ...`). Running a command like that sets the var for that one
# process; it doesn't export into your current shell. Run:
#     export GCLOUD_RW_API_KEY="glc_..."
# then check `echo ${#GCLOUD_RW_API_KEY}` again before continuing.

sudo /usr/libexec/PlistBuddy \
  -c "Delete :EnvironmentVariables:GCLOUD_RW_API_KEY" \
  /Library/LaunchDaemons/com.grafana.alloy.plist 2>/dev/null   # idempotent
sudo /usr/libexec/PlistBuddy \
  -c "Add :EnvironmentVariables:GCLOUD_RW_API_KEY string ${GCLOUD_RW_API_KEY}" \
  /Library/LaunchDaemons/com.grafana.alloy.plist

sudo plutil -lint /Library/LaunchDaemons/com.grafana.alloy.plist

sudo launchctl bootout system/com.grafana.alloy
sudo launchctl bootstrap system /Library/LaunchDaemons/com.grafana.alloy.plist
```

The `Delete` before `Add` keeps it safe to rerun — PlistBuddy's `Add` fails if the key already exists. `plutil -lint` catches any syntax corruption before you load the plist. `bootout` + `bootstrap` restarts the daemon so it picks up the new env.

## Verify the fix worked

```bash
# Wait a few seconds for the remote_write queue to drain and retry.
sleep 5

# Check stderr: the 401 spam should stop.
tail -30 /var/log/alloy/stderr.log

# failed_total should plateau. samples_total should keep climbing.
curl -s http://127.0.0.1:12345/metrics \
  | grep -E 'prometheus_remote_storage_samples_(failed_total|total)' \
  | head -5

# Doctor still all-green.
spawn-claude doctor
```

Then, once Claude emits a batch of telemetry (run `spawn-claude run -- -p "hello"` if you want to force it), open Grafana Cloud → Explore → your Prometheus datasource and run:

```promql
count by (__name__) ({__name__=~"claude_code.*"})
```

You should see names like `claude_code_token_usage_total`, `claude_code_session_count_total`, etc. If yes — done.

## Persistence across reboots

**Yes, this survives reboots.** `launchd` reads `/Library/LaunchDaemons/*.plist` on every boot. `RunAtLoad=true` in the plist means Alloy starts immediately. The `EnvironmentVariables` dict is applied to the daemon's process env at launch. The secret lives in the plist on disk, so it persists until you explicitly remove it.

**It does NOT survive a reinstall.** `spawn-claude collector install` rewrites the plist from the embedded template, which does not contain your `GCLOUD_RW_API_KEY`. After any reinstall, rerun the PlistBuddy snippet above. This is deliberate: spawn-claude intentionally doesn't know or care about Grafana-Cloud-specific credentials, because Grafana owns that onboarding flow.

## Security note

macOS enforces mode `0644 root:wheel` on LaunchDaemon plists. If you `chmod 0600` the plist, `launchctl bootstrap` will refuse to load it. This means **any local user on the machine can read your `GCLOUD_RW_API_KEY`** by `cat`-ing `/Library/LaunchDaemons/com.grafana.alloy.plist`. That's strictly worse than having the secret in your `.zshrc` (mode `0600`, only your UID can read).

For a single-user laptop this is typically acceptable — an attacker with your user account already owns the secret anyway. For shared machines (lab hosts, team servers, etc.), use the `local.file` component approach instead: store the secret in a root-owned `0600` file that Alloy (also running as root) can read, and reference it from the config:

```bash
# As root:
sudo mkdir -p /etc/alloy/secrets
sudo tee /etc/alloy/secrets/gcloud_rw_api_key >/dev/null <<< "$GCLOUD_RW_API_KEY"
sudo chmod 0600 /etc/alloy/secrets/gcloud_rw_api_key
sudo chown root:wheel /etc/alloy/secrets/gcloud_rw_api_key
```

Then swap both `password = sys.env("GCLOUD_RW_API_KEY")` lines in your `/etc/alloy/config.alloy` for:

```hcl
local.file "gcloud_key" {
  filename  = "/etc/alloy/secrets/gcloud_rw_api_key"
  is_secret = true
}

// ... then inside both basic_auth blocks:
password = local.file.gcloud_key.content
```

Reload (`spawn-claude collector reload`) and the daemon picks up the new config without needing the env-var injection at all. You can remove the PlistBuddy-injected entry afterward.

## Related reading

- [ALLOY.md](ALLOY.md) § "Grafana Cloud onboarding deep-dive" — more on what `install-macos-binary.sh` actually does (and doesn't do), and why its final command references the wrong binary name.
- [MANUAL.md](MANUAL.md) § "Manual equivalent of `collector install`" — what spawn-claude's install does under the hood, step by step.
- Upstream Alloy docs on `sys.env`, `local.file`, and the remote_write/loki.write components: https://grafana.com/docs/alloy/latest/reference/components/
