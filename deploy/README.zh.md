# ProbeX Docker 部署

> 英文版见 [README.md](README.md)。

本目录支持两种部署路径：

1. 预构建镜像（推荐给使用者）
2. 本地构建镜像（推荐给开发者）

## 1）使用预构建镜像（不本地编译）

默认情况下，compose 文件拉取预构建镜像，不在本地编译源码。

```bash
cp deploy/.env.example deploy/.env
docker compose -f deploy/docker-compose.yml up -d
```

这会在 `:8080` 启动后端服务，但**不会**启动 `:3000` 上的 Vite 前端开发服务器。

分布式模式：

```bash
cp deploy/.env.example deploy/.env
docker compose -f deploy/docker-compose.distributed.yml up -d
```

如有需要，编辑 `deploy/.env` 设置不同的镜像 tag 或镜像加速地址。

分布式模式还可配置：

- `PROBEX_HUB_TOKEN`：hub 与 agent 之间的共享 token
- `PROBEX_HUB_WS_URL`：agent 连接地址，例如 `ws://<hub-host>:8080/api/v1/ws/agent`
- `PROBEX_AGENT_EAST_NAME`：agent-east 显示名
- `PROBEX_AGENT_WEST_NAME`：agent-west 显示名

## 2）本地构建镜像

当你确实想从本地源码构建时，使用 build override 文件：

```bash
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.standalone.build.yml up -d --build
```

分布式模式：

```bash
docker compose -f deploy/docker-compose.distributed.yml -f deploy/docker-compose.distributed.build.yml up -d --build
```

## 3）一体化部署（`docker-compose.probex.yml`）

`docker-compose.probex.yml` 在单机上同时运行**后端**（standalone）和一个 **nginx 托管的前端**。它会 bind-mount `../data`、`../configs/controller.yaml` 和 `../scripts/probes`（只读，用于脚本探针热更新）。

### 环境特有配置 → `docker-compose.override.yml`

每台主机特有的挂载、密钥、路径（如 ASR 数据目录、SSH 凭据等）应放进 `docker-compose.override.yml`，该文件**已被 gitignore**，不会污染共享的 compose 文件：

```yaml
services:
  backend:
    volumes:
      - /host/path/data:/host/path/data
      - ../configs/ssh:/root/.ssh:ro
```

由于主文件名是 `docker-compose.probex.yml`（不是默认的 `docker-compose.yml`），compose **不会**自动加载 override。因此每条命令都**必须同时传两个 `-f`**：

```bash
docker compose \
  -f deploy/docker-compose.probex.yml \
  -f deploy/docker-compose.override.yml \
  up -d --build backend frontend
```

对于**被 git 跟踪、但不应提交的**主机本地改动（例如 `configs/controller.yaml` 里主机特有的 `script_dir`），可以用 `git update-index --skip-worktree <文件>` 让它不被 `git pull` 覆盖。

### 更新流程

- **只改前端**：只重建 frontend，并加 `--no-deps`，这样 backend（以及正在进行的监测）**不会**被重启：
  ```bash
  docker compose -f deploy/docker-compose.probex.yml -f deploy/docker-compose.override.yml \
    up -d --build --no-deps frontend
  ```
  > 注意：frontend 在 compose 里 `depends_on: backend`，不加 `--no-deps` 会连带 recreate backend、中断监测几秒。
- **改了后端**：重建 `backend`（会重启，监测中断几秒）。如果重建后仍像跑的是旧代码，先确认源码是最新（`git rev-parse HEAD`），再 `build --no-cache backend`。
- bind-mount 的 `../data`（sqlite 数据库）和 `../configs/controller.yaml` 在 rebuild 后保留，不会丢。

### 脚本探针：热更新

`../scripts/probes` 是 bind-mount 的，所以修改某个脚本的**执行逻辑**会立即生效（每次运行都重新 exec 该文件）。**新增/删除脚本**，或**修改 `PROBEX_META` 元数据**，则需要重扫：在 Probes 页面点 **Rescan Scripts** 按钮，或调用 `POST /api/v1/probes/rescan`。无需重启。

## 本地开发时的前端

如果想在本地开发时使用 Web UI，单独运行前端：

```bash
cd web
npm install
npm run dev
```

然后打开：

- 前端：`http://localhost:3000`
- 后端 API：`http://localhost:8080/api/v1`
