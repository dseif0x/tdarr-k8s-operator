# tdarr-k8s-operator

A lightweight Kubernetes operator for [Tdarr](https://tdarr.io/) that deploys a
Tdarr **server** and provisions **on-demand transcode nodes** as Kubernetes
`Job`s only when there is work to do — then tears them down again when the queue
is empty. This is ideal for **GPU** nodes that you don't want sitting idle: the
operator gives every node Job your configured `runtimeClassName`, `tolerations`,
`nodeSelector`, `affinity` and `resources` (e.g. `nvidia.com/gpu: 1`).

## How it works

```
                        ┌─────────────────────────┐
                        │   Tdarr server (Deploy)  │  web UI :8265 / node :8266
                        │   internal node disabled │
                        └─────────────┬────────────┘
                                      │ polls /api/v2 queue + node status
                        ┌─────────────▼────────────┐
                        │   tdarr-operator (Deploy) │
                        │   reconcile loop          │
                        └─────────────┬────────────┘
            queue > 0 / busy          │          queue empty + idle > timeout
            create Job ───────────────┼─────────────── delete Job
                                      ▼
                        ┌──────────────────────────┐
                        │  Tdarr node (Job)         │  GPU runtimeClass /
                        │  connects to server :8266 │  tolerations / resources
                        └──────────────────────────┘
```

1. The Helm chart deploys the **Tdarr server** (with its built-in node disabled
   by default) plus the **operator**.
2. The operator polls the server every `controller.pollInterval`. It reads the
   transcode queue (`table1Count`) and health-check queue (`table4Count`) from
   the server statistics, plus the busy/idle state of connected node workers.
3. When there is pending work and no node Job exists, it creates a single node
   `Job` from the template rendered by the chart (this is where all your GPU
   scheduling settings live).
4. When the queue drains **and** the node has been idle for
   `controller.idleTimeout`, the Job (and its pod) is deleted.

A single node is provisioned on demand. The idle timeout prevents thrashing
between back-to-back files, and an in-flight transcode is never interrupted
because the operator also checks active node workers, not just the queue.

## Quick start

```bash
helm install tdarr ./charts/tdarr-operator \
  --namespace media --create-namespace \
  -f my-values.yaml
```

Minimal `my-values.yaml` for an NVIDIA GPU node with shared (RWX) storage:

```yaml
persistence:
  config:
    existingClaim: tdarr-config        # server config/db (RWO is fine)
  media:
    existingClaim: media-rwx           # ReadWriteMany – shared with nodes
  cache:
    existingClaim: transcode-cache-rwx # ReadWriteMany – shared with nodes

node:
  runtimeClassName: nvidia
  tolerations:
    - key: "nvidia.com/gpu"
      operator: Exists
      effect: NoSchedule
  resources:
    limits:
      nvidia.com/gpu: 1
```

Open the web UI:

```bash
kubectl -n media port-forward svc/tdarr-tdarr-operator-server 8265:8265
# then browse http://localhost:8265 and configure your libraries
```

> **Storage:** `media` and `cache` must be reachable by **both** the server and
> the node pods at the same paths. Because GPU nodes usually live on different
> physical hosts, use `ReadWriteMany` storage (NFS, CephFS, …). The `emptyDir`
> fallback only works for single-pod demos and will fail real transcodes.

## Configuration

See [`charts/tdarr-operator/values.yaml`](charts/tdarr-operator/values.yaml) for
the full set of options. Highlights:

| Key | Description | Default |
| --- | --- | --- |
| `server.image` | Tdarr server image | `ghcr.io/haveagitgat/tdarr` |
| `server.internalNode` | Let the server transcode too (no GPU knobs) | `false` |
| `node.image` | Tdarr node image | `ghcr.io/haveagitgat/tdarr_node` |
| `node.runtimeClassName` | RuntimeClass for node pods | `""` |
| `node.tolerations` | Tolerations for node pods | `[]` |
| `node.nodeSelector` / `node.affinity` | Node scheduling | `{}` |
| `node.resources` | Node resource requests/limits (GPU) | `{}` |
| `controller.pollInterval` | Server poll frequency | `15s` |
| `controller.idleTimeout` | Idle period before scale-down | `120s` |
| `persistence.{config,media,cache}` | Volumes (existingClaim / create / emptyDir) | see values |

## Repository layout

```
.
├── main.go                       # controller entrypoint
├── internal/
│   ├── config/                   # env-driven configuration
│   ├── tdarr/                    # minimal Tdarr HTTP API client
│   └── controller/               # reconcile loop (scale node Job up/down)
├── Dockerfile                    # multi-arch (amd64/arm64) controller image
├── charts/tdarr-operator/        # Helm chart (server + operator + node template)
└── .github/workflows/            # GHCR image build + Helm chart release
```

## Development

```bash
go test ./...                     # unit tests
go build ./...                    # compile
helm lint charts/tdarr-operator   # lint chart
helm template rel charts/tdarr-operator   # render manifests
```

## CI/CD

- **`docker-ghcr.yaml`** builds and pushes the multi-arch controller image to
  `ghcr.io/<owner>/<repo>` on pushes to `main` that touch Go sources or the
  Dockerfile.
- **`release-helm.yaml`** publishes the Helm chart via
  [`chart-releaser-action`](https://github.com/helm/chart-releaser-action) on
  pushes to `main` that touch `charts/**`.
