# bf-checks

Comp's integration checks (`packages/integration-platform`) packaged as one
standalone binary that Black Fortress runs on the developer's machine. The
check code is used unchanged; this package only adds a stdin/stdout protocol
and an adapter that turns local AWS credentials into the session shape Comp's
AWS checks accept.

```sh
bun install                 # also links Comp's sources to these dependencies
bun test && bun run typecheck
bun run build               # dist/bf-checks for this machine

bf-checks list              # providers and checks as JSON
echo '{"provider":"github","accessToken":"…","variables":{"target_repos":["acme/api:main"]}}' \
  | bf-checks run
```

`fortress/scripts/build-runtime.sh` cross-compiles it for each macOS target.
bfd discovers credentials, schedules runs and turns results into Probo
evidence; see `../ARCHITECTURE.md`.

| Provider | Credentials bfd uses | Checks |
|---|---|---|
| GitHub | `BF_GITHUB_TOKEN`, `GH_TOKEN`, `GITHUB_TOKEN`, or `gh auth token` | 5 |
| AWS | `AWS_*` env or `aws configure export-credentials` (long-term keys are exchanged for a session) | 9 |
| GCP | `BF_GCP_TOKEN` or `gcloud auth print-access-token` | 9 |
| Azure | `BF_AZURE_TOKEN` or `az account get-access-token` | 14 |
| Vercel | `BF_VERCEL_TOKEN` / `VERCEL_TOKEN` | 2 |
| Google Workspace | `BF_GOOGLE_WORKSPACE_TOKEN` (needs Admin SDK scopes) | 2 |
| Aikido | `BF_AIKIDO_TOKEN` | 3 |

License: AGPL-3.0, like the Comp code it bundles.
