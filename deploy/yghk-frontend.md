# yghk ProbeX Deployment

This runbook updates the existing Hong Kong validation instance, not a new
general-purpose deployment. Use the canonical `yghk` SSH alias and first read
the inventory and run preflight in the sibling PersonalServices repository.
Do not change regional-host roles or install build toolchains on the small VPS.

## Scope

- URL: https://yghk.tzpluster.com:8091/results
- Remote directory: `/root/codex-deployments/probex`
- Compose file: `docker-compose.yml`; service: `frontend`
- Containers: `probex-hk-frontend`, `probex-hk-backend`
- Frontend publishes `8091:443`, serves static assets and proxies `/api/` and
  `/health` to `backend:8080` on the existing Docker network.
- Preserve remote `nginx.conf`, read-only certificate mount, `.env`, controller
  configuration and `data/`. Never copy secrets into build contexts or logs.
- Frontend-only releases must not restart the backend. Do not run compose down
  or automatically clear results. During explicitly authorized v4 schema iteration,
  obsolete test history cleanup is a separate, backed-up transaction after target
  and record verification; it is never a startup or deployment-script side effect.
  An explicitly requested coordinated backend release follows the procedure below instead.

## Coordinated Backend Release

Use this procedure for an explicitly requested coordinated backend and UI release.
The backend must be upgraded before the extension. There is no database migration
in the releases recorded here, and history cleanup is never an automatic part of
this procedure.

1. Complete the inventory/preflight and conflict checks above. Record both
   container IDs, image IDs, deployment configuration hashes, mounts, network,
   firewall state and record counts. Keep credentials on the remote host only.
2. Run Go race tests, vet and Web/extension checks locally. Cross-compile the
   backend with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath
   -ldflags="-s -w" -o <release>/backend/probex ./cmd/probex`; build Web locally.
   Record source branch/base commit/dirty status and artifact SHA-256 values.
3. Under a unique remote release directory, retain the previous backend binary,
   frontend assets, rollback image tags and an online SQLite backup using the
   SQLite backup API (not a raw copy of a live WAL database). Restrict the release
   directory and backup to the operator. Do not include data/configuration in
   image build contexts or transfer it off the host.
4. Upload only the new executable, static assets and non-secret release metadata.
   Build two COPY-only images from the exact previous images, preserving their
   runtime dependencies/scripts/configuration and the frontend's old hashed assets.
   Validate the executable with `--help` without host mounts/network, and validate
   Nginx with its existing network and read-only certificate mount, without ports.
5. Tag both release images with their existing `latest` tags. Recreate only
   `backend`, then `frontend`, each with `docker compose up -d --no-deps
   --no-build --pull never <service>`. Wait for backend health before recreating
   the frontend; this also refreshes Nginx's upstream DNS after backend replacement.
   Expect a brief API interruption. Do not change other workloads.
6. Verify HTTPS routes/assets and authenticated read APIs, on-disk/container/served
   hashes, unchanged configuration/mounts/ports, and preserved pre-release records
   and registrations (agent liveness timestamps may naturally change; compare
   agent plugin lists as sets, since registry enumeration order may change). Use a
   rejected invalid `result_id` request to verify new validation without inserting
   synthetic records into the live database. Replay/dedup tests use isolated stores.
7. Update remote rebuild artifacts to the verified binary and static assets.
   Keep release/rollback artifacts and the database backup; remove only this
   release's disposable transfer archive. Update this runbook and PersonalServices
   service/index records, and run `python3 check.py` there.
8. If verification fails, retag both saved rollback images as `latest`, recreate
   backend then frontend and verify health again. Restore prior rebuild artifacts,
   not the live database: there is no schema migration, and automatic database
   restore would discard records arriving after the backup.

Chrome extension/page reload is separate. Verify the unpacked path and matching
manifest version (currently 1.1.5), reload after backend health passes, and reload only idle
GuideX pages. Do not toggle Auto-Test or start a real microphone interaction as
part of deployment. A fresh user conversation is required for end-to-end acceptance.

## Release Procedure

1. Check container IDs, image IDs, start times, published ports, mounts,
   reverse-proxy configuration, firewall and `/health` before changing anything.
   Compare mount lists sorted by destination: Docker inspect does not guarantee
   their order. Normalize firewall counters and generated comments before comparison.
2. Locally run `node --test tests/guidex-timeline.test.mjs`, `npm run build`
   and targeted ESLint from `web/`. Upload only `web/dist/` to a uniquely named
   remote `releases/<release>/dist/`, never the full workspace or `.env`.
3. Tag the currently running frontend image as `probex-hk-frontend:rollback-<release>`.
   Back up remote `dist/` in the release directory and record the source branch,
   base commit, dirty-worktree status and asset checksums. A dirty-worktree build
   is not reproducible from the base commit alone; retain its release artifacts.
4. Build a COPY-only image from that exact rollback image, replacing static
   assets only. This preserves deployed Nginx/TLS configuration and previous
   hashed assets for tabs still using the previous entrypoint. Do not pull a
   floating base image or compile Node/Go on yghk.
5. Validate the image with `nginx -t` on the existing Docker network and the
   existing read-only certificate mount, without publishing another port.
6. Tag the release image as `probex-hk-frontend:latest`, then run from the remote
   directory:

   ```sh
   docker compose up -d --no-deps --no-build --pull never frontend
   ```

7. Verify HTTPS `/results`, each referenced asset, `/health`, `/api/v1/mode`
   and authenticated read-only results queries. Compare served checksums with
   the uploaded build. Confirm the backend container ID and start time did not
   change. Refresh the browser results page; do not reload an active GuideX
   conversation as part of frontend deployment.
8. Update remote `dist/` to the verified artifacts for future rebuilds; retain
   the previous image and artifact backup for rollback. Remove only this
   release's disposable transfer archive after verification. Do not prune
   unrelated images, files or data.
9. Update `PersonalServices/services/yghk-probex.md` and indexes, then run
   `python3 check.py` there. Record failures and rollback explicitly.

## Frontend-Only Rollback (Historical)

For this release, `<release>` is `20260910T032218Z`:

```sh
cd /root/codex-deployments/probex
docker tag probex-hk-frontend:rollback-20260910T032218Z probex-hk-frontend:latest
docker compose up -d --no-deps --no-build --pull never frontend
```

Recheck HTTPS and backend health. Restore the prior entrypoint and assets from
`releases/20260910T032218Z/previous-dist/` to `dist/` before a later rebuild.
No database rollback or history deletion is part of this operation.

## GuideX Version Boundaries

The v4 UI expects `guidex-runtime-v4` results from the Runtime v4 extension and
GuideX `/#/interaction-app/...`. Legacy `guidex-interaction` and WebRTC remain
separate. An empty v4 task may not appear in the result selector until a fresh
result arrives. Do not insert synthetic results into the live database just to
display the timeline. Extension/page reloads and protocol selection are separate
from this frontend release; deploying this UI does not upgrade GuideX itself.

Fields are not finalized. Follow the repository's schema lifecycle instructions;
a deployment without a new field change does not trigger history cleanup.

## Release Record

### History Repair: 20260911T071419Z-history-repair

- 2026-09-11 07:17:02 UTC（北京时间 15:17:02）按用户明确确认，修正 14 条
  Runtime v4 历史 `session_ended` 失败记录。先前确认的 14:16:00 两条和另
  12 条均由用户确认是人脸消失后的正常离场；旧记录本身未保存协议结束原因，
  本次依据是人工确认，不是仅凭 `session_ended` 或 `guidance` 自动推断。
- 唯一修改字段是顶层 `success=1` 和 `extra.success=true`，按固定 14 个
  result ID 在事务内修复。`completion_reason=session_ended`、原始时间点、
  STT/回答等字段保持原值，`latency_ms/total_interaction` 原本就是空值，
  不补造节点、不新增原因字段、不删除历史、不增加自动改判逻辑。
- 修复前已使用 SQLite Backup API 在线备份并通过 quick_check；以 34 条 v4
  数据进行隔离模拟修改和回滚验证。事务内结果总数 148,708 不变，v4 成功数
  从 20 变为 34，其他表内容及非目标 v4 记录不变；四级汇总原本均为 0。
- 07:17:08 UTC HTTPS 认证原始查询/JSON 导出各 34 条，趋势聚合 20 条，
  固定历史窗口成功率为 100%。14 条修复记录保留在明细/原始导出，从趋势
  聚合和既有单轮筛选中排除。核对备份中所有非目标历史结果未变、未丢失；
  全部容器及配置不变，health 正常，验收时其他结果自然增长至 148,720。
- 受限远端目录 `releases/20260911T071419Z-history-repair/` 保留
  `before-db.sqlite`、`repair.py`、修复前后目标记录、`prepared.json`、
  `applied.json` 和 `verification.json`。备份和原始记录未下载到本机。
  如需撤销，须另行授权并核对当前记录与修复后快照相同，只恢复这 14 条的
  两个成功标记；不要恢复整库或覆盖新结果。
- 用户已反馈重载 Chrome；本次没有操作插件/GuideX 页面，也不冒充已完成
  新采样验收。此次为数据修复，无需重新部署或再次重载插件，刷新 Results 即可。

### Current: 20260911T065446Z

- 后端/前端协调发布，2026-09-11 07:05:49 UTC（北京时间 15:05:49）最终验收通过。
  后端在聚合前排除 `success=true + completion_reason=session_ended` 的
  Runtime v4 正常收尾；前端单轮选择器同步排除，明细及独立原始 Excel 导出保留。
  没有图表记录时 Export 仍可用。插件 1.1.5 负责确认 `presence_left` 正常离场，
  保留已观测节点，`total_interaction/latency_ms` 为空；异常及未知原因仍失败。
- 两个镜像都在本机构建产物、远端 COPY-only 封装，没有在 yghk 编译 Node/Go。
  后端镜像 `probex-hk-backend:release-20260911T065446Z`，镜像 ID
  `sha256:b6617000021d6606c4f4c103b52c9cc42e97112a6d1fd56cb3f8116371dc754b`；
  前端镜像 `probex-hk-frontend:release-20260911T065446Z`，镜像 ID
  `sha256:ce72d0a0b7078241e60da10b503fd0b10d7863a82c7ae6c37b0c7e8ad591b71a`。
  两者同时标记 `latest`；旧哈希静态资源保留。
- 后端容器 `bb2a8e01c4a909b8c8fbe3ca35bfc027bb235e0763e9a40c5f5f2247a8ca8bad`，
  启动 `2026-09-11T07:05:40.955202376Z`；前端容器
  `7abcfad200d86590a24c05cb59f73132eb96eef32546fa1f96161585fbe5c1c1`，
  启动 `2026-09-11T07:05:41.979984313Z`。
- 后端二进制 SHA-256：
  `e7e5db04014300df25b0d234f7cc6ffd4dedc78677b75a2b5885ab4f89612fae`。
  主资源 `/assets/index-DwVJ9mqB.js`，SHA-256：
  `67403c252d56a8686130785a94d7e2b5d97b783e5ec6f948b09019fca203e70f`。
  来源为两个适配分支的未提交工作区；源文件及产物摘要均在 `manifest.json`。
- 本机插件 87 项及语法检查、前端 21 项及构建、Go 全量 race/vet 通过；
  新工具函数/单轮组件定向 ESLint 通过。Results 全文件仍有既有 any/表达式/
  Hook 依赖问题，构建有既有大分包警告，未在部署时扩展修改源码。
  上一轮隔离浏览器已验证正常离场明细/导出可见、两个图表排除及异常轮次保留。
- 远端 HTTPS/SPA、全部产物及容器/线上哈希、Nginx、backend/HTTPS health、
  standalone mode、未授权 401、认证读取和 v4 聚合选择验证通过。
  验证原始与聚合各 34 条现有 v4 记录；这些记录尚非新版正常离场样本，
  不将本次资源/接口验收冒充新版真实交互端到端验收。
- 发布前 146,786 条结果，成功验收时 147,430 条；34 条 v4、7 个任务、
  7 个探针和 1 个 Agent 保留。备份与线上逐行验证全部原始结果未改写/丢失；
  Agent 能力集合未变。其他容器、配置、端口、挂载、网络和规范化防火墙摘要未变。
  本次没有字段键变化，不清理或改判历史，也不新增自动清理逻辑。
- 前两轮验收曾自动回滚：首轮验收脚本局部 import 导致 urllib 作用域错误；
  第二轮因 Agent 插件数组排列变化误报注册变更。核实只是相同能力集合的顺序
  变化后改为排序比较，第三次验收成功。两次回滚均验证旧镜像/健康/配置恢复，
  无整库恢复；失败、回滚、重试状态及各版脚本保留，不省略发布尝试记录。
- `releases/20260911T065446Z/` 保留受限 SQLite Backup API 在线备份
  `previous-db.sqlite`（quick_check=ok）、`previous-probex`、`previous-dist/`、
  精确发布物、构建/验证/尝试日志及 `verification.json`、`postflight.json`。
  远端重建源 `probex` 和 `dist/` 已同步，传输压缩包已删除；备份不下载到本机。
- 07:06:55 UTC 最终复核服务健康、容器未再变化、产物及配置一致，结果自然增长至
  147,532，v4 仍 34。`collector_schema_current=false`。Chrome 核对为 1.1.4，
  加载来源 `~/workspace/network/probex-webrtc-guidex-extension` 正确；随后有用户
  操作，停止浏览器动作，没有重载插件或 GuideX。须重载至 1.1.5 并刷新空闲
  GuideX 页面后采样，Results 也需刷新；没有触发真实交互或 Auto-Test。

当前协调回滚使用两个 `rollback-20260911T065446Z` 标签，按上方协调流程
依次恢复 backend、验证其健康、再恢复 frontend。后端基线镜像
`sha256:6440555f8aae393c1aa6ae690410c6e490dd27242181204e158e84acc3a5036f`，
前端基线镜像
`sha256:546d52d5d07c603ddb73b603a8eb78dc067926df8d9d048fd607876bfba12d9e`。
回滚后将本 release 的 `previous-probex`、`previous-dist/` 恢复为重建源；
不要恢复整个数据库，也不要影响其他服务。

### Previous: 20260911T043644Z

- 前端-only 发布，2026-09-11 04:39:14 UTC（北京时间 12:39:14）验收通过。
  移除 `Audio_To_Speech` 展示，保留 `Speech_Started`；两个时长短名改为
  `Answer_Dur`、`Play_Dur`，数据键和公式不变。三个 `LastAudio_To_*`
  保留，当前图表为四个早期节点和七个间隔。
- 镜像：`probex-hk-frontend:release-20260911T043644Z`，同时标记 `latest`；
  镜像 ID：`sha256:546d52d5d07c603ddb73b603a8eb78dc067926df8d9d048fd607876bfba12d9e`。
  frontend 容器：`d635c1c9a0b1f1ae9d1db78f6732c440d83855419d739e46892828e560956b9a`，
  启动时间：`2026-09-11T04:39:07.005339346Z`。
- 主资源 `/assets/index-dFDR2hW_.js`，SHA-256：
  `1646178c35c1ba2257c7588e0238cecc0397810d4b37bc35d9c4c6cee02b2f7d`。
  `index.html` SHA-256：
  `409d528eff51c078c9f05dbc6e1bff3856d0fb1c670c7bbe6d0989090e1887d4`。
  本机构建、远端 COPY-only 封装；CSS/xlsx 分包未变，旧哈希资源保留。
- 来源：`codex/guidex-v4-timeline`，基线
  `73ae2e7468c34409cd456947e6e8c027c9c87138` 加未提交改动。
  前端 17 项、插件 78 项测试、生产构建、定向 ESLint 和插件语法检查通过。
  构建仍有既有的大于 500 kB 分包提示。HTTPS/SPA、全部资源哈希、Nginx、
  health/mode、认证只读查询及未授权拒绝验证通过。
- 独立浏览器选择 Runtime v4 专属结果及最近 24 小时，核对趋势图、表格、
  底部单轮图的新短名；不是 All Tasks 页面。最近一小时没有样本，不能据此
  判断发布失败。浏览器验证使用清理前记录，不代表新版真实交互验收。
- backend 容器仍为
  `a1aee059f7011c1e1d4b60fbe201abc3a352d8847b26b33a375535c2134db0a9`，
  启动时间仍为 `2026-09-10T11:23:10.963575184Z`；其他容器、配置、端口、
  挂载、网络及规范化防火墙摘要未变。发布前 137,172 条结果，发布验收
  137,206 条，全部原有结果保留；发布一次成功，未触发回滚。
- 按未定版字段规则，04:41:44 UTC 在独立事务中仅删除
  `ext_guidex-runtime-v4` 的 3 条旧格式结果；四级汇总原本均为 0。
  目标集合及内容与 SQLite 在线备份一致才执行，清理后原始记录及汇总均为 0。
  同一事务内其他 137,284 条结果及全部非目标表计数和内容摘要未变；
  7 个任务、4 个外部探针和 1 个 Agent 注册保留。没有自动清理逻辑。
- `releases/20260911T043644Z/` 保留受限的 `previous-db.sqlite`
  （SQLite Backup API，quick_check=ok）、`previous-dist/`、精确发布物、
  `manifest.json`、`preflight.json`、`verification.json`、
  `schema-cleanup.json`、`postflight.json` 和发布日志。备份未下载到本机。
  远端 `dist/` 已同步；本次传输压缩包已删除，不清理其他发布资源。
- 04:42:28 UTC 最终检查：健康正常、资源/配置一致、发布后容器未变；
  v4 原始记录和四级汇总均为 0，认证查询为空，总结果自然增长至 137,308。
  `collector_schema_current=false`，服务器注册尚不满足插件 1.1.4 字段。
  本次未操作 Chrome、重载插件/GuideX 或触发真实交互/Auto-Test。
  须重载 1.1.4 插件并刷新空闲 GuideX 页面后采样；Results 也需刷新。
  字段仍未定版，新样本端到端验收尚未完成。

当前 release 回滚（只恢复前端，不恢复数据库）：

```sh
cd /root/codex-deployments/probex
docker tag probex-hk-frontend:rollback-20260911T043644Z probex-hk-frontend:latest
docker compose up -d --no-deps --no-build --pull never frontend
```

回滚基线镜像为
`sha256:8c5394df7aca231b84de9c7a3290e929ef0e67c374392382c8c462807e747cd7`。
回滚验证后，将 `releases/20260911T043644Z/previous-dist/` 恢复到 `dist/`
作为后续重建源。不要恢复整库或重启 backend；受控旧测试清理不随镜像回滚撤销。

### Previous: 20260911T032612Z

- 前端-only 发布，2026-09-11 03:30:25 UTC（北京时间 11:30:25）验收通过。
  三个间隔改为 `LastAudio_To_STT`、`LastAudio_To_Answer`、`LastAudio_To_Play`，
  统一读取新 `last_audio_to_*` 数据键；旧 `Stop_To_*` 和独立 `Test_To_Play`
  从图表、表格及导出中移除，不将旧值改名或反推为新指标。
- 镜像：`probex-hk-frontend:release-20260911T032612Z`，同时标记 `latest`；
  镜像 ID：`sha256:8c5394df7aca231b84de9c7a3290e929ef0e67c374392382c8c462807e747cd7`。
  frontend 容器：`3d6c61945a455a4b2b9947bc3fb3ed66b470e39ab8166b2298b4637c9b553a47`，
  启动时间：`2026-09-11T03:30:16.497900474Z`。
- 主资源 `/assets/index-CTRu6mFT.js`，SHA-256：
  `614899527c34ea74671d08052664c0d03fd688ddfe155b1de6bef506f9ee7182`。
  本机构建、远端 COPY-only 封装，保留旧哈希资源；没有在 yghk 编译 Node/Go。
- 来源：`codex/guidex-v4-timeline`，基线
  `73ae2e7468c34409cd456947e6e8c027c9c87138` 加未提交改动。
  前端 16 项、插件 78 项测试及生产构建、定向 ESLint 通过。
  HTTPS/SPA、全部资源及容器内哈希、认证查询、未授权拒绝、配置/端口/
  挂载/防火墙检查通过；独立浏览器在 Runtime v4 专属结果页核对了趋势图、
  表格、底部单轮图的新名称。旧记录的新间隔为空，未做兼容推算。
- backend 容器保持
  `a1aee059f7011c1e1d4b60fbe201abc3a352d8847b26b33a375535c2134db0a9`，
  启动时间仍为 `2026-09-10T11:23:10.963575184Z`；其他容器未变化。
  发布前 133,597 条结果，切换验收时 133,641 条，发布本身保留全部原有结果。
- 随后按未定版字段清理规则，于 03:32:31 UTC 单独事务性删除
  `ext_guidex-runtime-v4` 的 11 条旧口径测试记录，四级汇总原本均为 0。
  按备份中的确切记录集合和内容核验后执行，清理后目标原始记录及汇总均为 0。
  同一事务内验证其他 133,742 条结果及所有非目标表内容摘要未变；
  7 个任务、4 个外部探针和 1 个 Agent 注册保留。没有自动清理任务。
- `releases/20260911T032612Z/` 保留 SQLite Backup API 在线备份
  `previous-db.sqlite`（quick_check=ok）、`previous-dist/`、精确发布物、
  源/资源哈希清单、`verification.json`、`schema-cleanup.json` 和
  `postflight.json`。远端 `dist/` 已同步；本次上传压缩包已删除。
  发布一次成功，未触发回滚；备份仅留在受限远端目录，不下载到本机。
- 本次未重载本机 Chrome 插件或 GuideX 页面。配套插件源码为 1.1.3，
  远端探针注册仍缺少三个新字段，需重载插件并刷新空闲 GuideX 页面后
  重新采样。未触发真实对话或 Auto-Test，尚未完成新数据的端到端验收。
  Results 需刷新；无 v4 记录时需等待新采样，不能填充模拟数据。字段仍未定版。

当前 release 回滚（只恢复前端，不恢复数据库）：

```sh
cd /root/codex-deployments/probex
docker tag probex-hk-frontend:rollback-20260911T032612Z probex-hk-frontend:latest
docker compose up -d --no-deps --no-build --pull never frontend
```

回滚验证后，将 `releases/20260911T032612Z/previous-dist/` 恢复到 `dist/`
作为后续重建源。不要恢复整库或重启 backend；本次受控旧测试清理不随镜像回滚撤销。

### Previous: 20260910T163003Z

- 前端-only 发布，2026-09-10 16:35:53 UTC（北京时间 2026-09-11
  00:35:53）验收通过。当前 Runtime v4 图表、表格及导出短名不带编号，
  原 `_Dur` 短名改用 `_To_`，保留 `1st`；字段键、计时公式和业务顺序不变。
- 镜像：`probex-hk-frontend:release-20260910T163003Z`，同时标记 `latest`；
  镜像 ID：`sha256:9d458640a5a1dbfbd8d1df29ddabc4737affc79f65521788ac97a998abdb8afb`。
  frontend 容器：`96866a3e1e2920da0eb96c9a28cfb384dc81da8a4bf96c895a9b51631c2a3428`，
  启动时间：`2026-09-10T16:35:51.790761491Z`。
- 主资源 `/assets/index-XJbJz1gk.js`，SHA-256：
  `e15878e7c3f89e8d9fae68a55d07e882d53e1e43b363dba694cc175679f7d999`。
  CSS 和 xlsx 分包未变化；旧哈希资源保留，当前入口已更新。
- 来源：`codex/guidex-v4-timeline`，基线
  `73ae2e7468c34409cd456947e6e8c027c9c87138` 加本地未提交改动。
  本机前端 15 项、插件 74 项测试通过；生产构建和定向 ESLint 通过。
  精确发布物、源清单、旧 dist、回滚镜像及验收记录保留在 release 目录。
- HTTPS/SPA、全部静态资源哈希、health/mode、认证只读查询、未授权拒绝、
  端口/挂载/配置/防火墙和历史记录保留验证通过。发布前 95,183 条结果，
  验收时 95,187 条；原有结果全部保留，v4 9 条，7 个任务、4 个探针和
  1 个 Agent 未减少。backend 容器及 `2026-09-10T11:23:10.963575184Z`
  启动时间不变；未清库、未恢复数据库。
- 首次切换因挂载数组顺序不同而误报其他容器变化，并自动回滚前端。
  已确认容器 ID、启动时间和挂载内容均未变化；按目标路径排序后重新
  验收通过。首次失败和回滚记录保留，不将其误记为业务服务故障。
- 独立浏览器选择 `guidex-runtime-v4 [external]` 和最近 24 小时，确认
  趋势图、单轮图及表格使用无编号的新名称。Chrome 存在用户操作，未强制
  刷新其现有标签页；需刷新 Results 才能载入新版脚本。
- 本次只发布 ProbeX 前端。插件注册说明仍需重载本机插件并刷新空闲
  GuideX 页面后更新；未重载采集页面、未触发测试交互。字段仍未定版。

当前 release 回滚（不恢复数据库）：

```sh
cd /root/codex-deployments/probex
docker tag probex-hk-frontend:rollback-20260910T163003Z probex-hk-frontend:latest
docker compose up -d --no-deps --no-build --pull never frontend
```

回滚验证后，将 `releases/20260910T163003Z/previous-dist/` 恢复到 `dist/`
作为后续重建源。不要恢复数据库或重启 backend。

### Previous: 20260910T122654Z

- 前端-only 发布标识：`20260910T122654Z`，2026-09-10 12:35:33 UTC
  完成容器重建和线上验收。仅重建 `frontend`，backend 容器 ID 和启动时间
  保持不变；未执行数据库操作或历史清理。
- Frontend：`probex-hk-frontend:release-20260910T122654Z`，同时标记
  `latest`；镜像 ID
  `sha256:fc2f3266713ce482de41b97d26261530f9ed65acc98737327e467e37cbd69f7a`。
  容器 ID
  `367285f4de5d98cdc2a387fd96bd38ed24ae95d8515cc8ef2f012e284efc3761`，
  启动时间为 `2026-09-10T12:35:33.048848612Z`。
- 这是从当前线上镜像制作的 COPY-only 前端镜像，回滚基线为
  `probex-hk-frontend:rollback-20260910T122654Z`，基线镜像 ID
  `sha256:a3196e1f1791b8997629460504810456aa7f5bd240faf376fab18ff0caac49d5`。
  Nginx 配置检查通过，未拉取浮动基础镜像或在 yghk 编译 Node。
- 本次前端包含“早期节点 + 关键间隔周期”图表：保留 1-4 节点，后续改为
  `Audio_Speech_Dur`、`Stop_STT_Dur`、`Stop_Answer_Dur`、
  `STT_Answer_Dur`、`Answer_Dur`、`Stop_Play_Dur`、`Play_Dur` 和
  `Interrupt_ACK`；表格和导出仍保留完整字段。
- 主资源 `/assets/index-BztvPoOA.js` 的 SHA-256 为
  `5426affc0eb0ff17553f7ffe79ab0ceb6c335f9b000194d5fdb1e01c74433800`；
  CSS `/assets/index-BU0rcNAY.css` 的 SHA-256 为
  `bccba430480dcaf0d86962c55a2e470743d2d90950fc32e873dd69d125819c9e`；
  `xlsx` 资源 `/assets/xlsx-B7Fe_CV5.js` 的 SHA-256 为
  `4124d76d6ed7fc8cab79c09a32475cdc4c677b7f2d962e3e2a0dc040bde9a72d`。
- 来源：ProbeX `codex/guidex-v4-timeline`，基线
  `73ae2e7468c34409cd456947e6e8c027c9c87138` 加本地未提交改动；本机
  `npm run build` 和 GuideX 时间线 8 项测试通过。定向 ESLint 仍受仓库既有
  `no-explicit-any` 问题影响，但未阻断生产构建。
- HTTPS `/results`、入口及全部静态资源、`/health`、`/api/v1/mode`、
  未授权读接口拒绝、认证只读结果查询、容器内静态资源哈希和 Nginx 配置
  均验证通过。认证结果查询返回结构正常，`ext_guidex-runtime-v4` 当前
  查询到 7 条记录；本次未向数据库写入或清理记录。
- backend 保持为容器
  `a1aee059f7011c1e1d4b60fbe201abc3a352d8847b26b33a375535c2134db0a9`，
  启动时间 `2026-09-10T11:23:10.963575184Z`。远端
  `probex/dist` 已更新为本次经验证的产物，发布目录保留
  `previous-dist/`、回滚镜像、清单和 `verification.json`。
- Chrome 插件和已注入的 GuideX 页面不随 ProbeX 前端发布更新；需单独重载
  插件并刷新空闲 GuideX 页面后，才能验收新的 Runtime v4 交互。字段仍未定版，
  本次部署不代表用户确认定版。

当前 release 回滚（不恢复数据库）：

```sh
cd /root/codex-deployments/probex
docker tag probex-hk-frontend:rollback-20260910T122654Z probex-hk-frontend:latest
docker compose up -d --no-deps --no-build --pull never frontend
```

回滚验证后，将 `releases/20260910T122654Z/previous-dist/` 恢复到
`dist/` 作为后续重建源。不要恢复数据库，也不要重启 backend。

### Previous: 20260910T111536Z

- 协调发布标识：`20260910T111536Z`，2026-09-10 11:23:16 UTC
  （北京时间 19:23:16）完成服务端验收。backend/frontend 容器分别于
  11:23:10/11:23:12 UTC 启动。
- Backend：`probex-hk-backend:release-20260910T111536Z`，同时标记
  `latest`；镜像 ID `sha256:6440555f8aae393c1aa6ae690410c6e490dd27242181204e158e84acc3a5036f`。
- Frontend：`probex-hk-frontend:release-20260910T111536Z`，同时标记
  `latest`；镜像 ID `sha256:a3196e1f1791b8997629460504810456aa7f5bd240faf376fab18ff0caac49d5`。
- Backend 二进制 SHA-256：
  `7bb54d01828e2a624e43d5b2b116df9bad829301b3a08759da063b718663fbb6`。
  主资源 `/assets/index-BhdeYlZH.js` 的 SHA-256 为
  `1bdf2cb4e19a2c2d6c5e9199dc69e7a7b93337a188aaa7894a74e05b810b1fc5`；
  `xlsx` 资源 `/assets/xlsx-B7Fe_CV5.js` 的 SHA-256 为
  `4124d76d6ed7fc8cab79c09a32475cdc4c677b7f2d962e3e2a0dc040bde9a72d`。
- 来源：ProbeX `codex/guidex-v4-timeline`，基线
  `73ae2e7468c34409cd456947e6e8c027c9c87138` 加本地未提交改动；在本机
  使用 Go 1.26.5、CGO disabled 交叉构建 Linux/amd64 backend，并使用本机
  Web production build。精确发布物和哈希保留在远端 release 目录。
- HTTPS `/results`、SPA 资源、`/health`、`/api/v1/mode`、认证只读 API、
  非法 `result_id` 拒绝、容器内二进制哈希和 Nginx 配置均验证通过。
  发布时结果数为 81,010 → 81,016，独立 postflight 时为 81,122；7 个任务、
  4 个探针和 1 个 Agent 均未减少。未清理历史记录，未执行数据库恢复。
- `releases/20260910T111536Z/` 保留 SQLite Backup API 生成的
  `previous-db.sqlite`（`quick_check=ok`）、`previous-probex`、
  `previous-dist/`、回滚镜像、源清单和校验记录；远端 `probex/dist` 已更新为
  本次经验证的重建产物。没有执行防火墙变更，规范化 postflight 摘要保存在
  `verification.json`。
- Chrome 插件和已注入的 GuideX 页面仍待单独重载；ProbeX 服务部署不会自动
  更新插件。Runtime v4 字段仍未定版，本次部署不代表用户确认定版。

此 release 回滚（不恢复数据库）：

```sh
cd /root/codex-deployments/probex
docker tag probex-hk-backend:rollback-20260910T111536Z probex-hk-backend:latest
docker tag probex-hk-frontend:rollback-20260910T111536Z probex-hk-frontend:latest
docker compose up -d --no-deps --no-build --pull never backend
# Wait for backend health before recreating frontend.
docker compose up -d --no-deps --no-build --pull never frontend
```

回滚验证后，将 `previous-probex` 恢复到 `probex`，将
`previous-dist/` 恢复到 `dist/` 作为后续重建源。不要自动恢复数据库备份，
以免丢失备份之后新产生的记录。

### Previous: 20260910T083953Z

- Coordinated backend/frontend release verified on 2026-09-10 at 08:49:14 UTC.
- Backend: `probex-hk-backend:release-20260910T083953Z`, also tagged `latest`.
  Image ID: `sha256:b847b5d355624da222044e143d38be27971e8c8e54885b6c4e64d7fcc5dcbed5`.
- Frontend: `probex-hk-frontend:release-20260910T083953Z`, also tagged `latest`.
  Image ID: `sha256:976df46c0c42ec4b78a72a88adb74ed8cba698bb34d7471095e57b1949bde7b6`.
- Backend binary SHA-256: `7bb54d01828e2a624e43d5b2b116df9bad829301b3a08759da063b718663fbb6`.
- Main asset: `/assets/index-C5F8T51t.js`; SHA-256:
  `109081c5222ccba7b5c5cfacdaca42d03939ab74212c6a2fe673c4269d8c95ee`.
- Source: ProbeX `codex/guidex-v4-timeline` at base
  `73ae2e7468c34409cd456947e6e8c027c9c87138` plus uncommitted changes.
  Built locally with Go 1.26.5 for Linux/amd64 (static, CGO disabled).
  Exact artifacts and hashes are retained, not inferred from the base commit.
- Verified all served assets, SPA routes, health/mode and authenticated reads.
  An isolated, network-disabled temporary container accepted one sample and
  counted its replay as a duplicate, leaving exactly one row. It was removed.
  Live invalid-ID validation returned 400 without inserting a test record.
- All 74,824 pre-backup results, seven tasks and four external registrations were
  preserved; live collection continued, reaching 75,270 results at verification.
  Agent identity/registration remained intact; heartbeat/last-push timestamps
  may advance. No migration, manual deletion or database restore was performed.
- Configuration hashes, credentials, mounts, ports, restart policies and unrelated
  containers were unchanged. Firewall rules matched after excluding generated
  timestamps and packet/byte counters from `iptables-save` output.
- Attempt 1 was rolled back after a verification false positive compared the
  naturally changing `probes.last_push_at`. Corrected verification excludes that
  liveness field but compares all registration definitions and historical results;
  attempt 2 passed. Both attempts and the final report are retained in the release
  directory, including `deployment-failed.txt` and `verification.json`.
- `releases/20260910T083953Z/` retains `previous-db.sqlite` (SQLite backup API,
  quick-check passed), `previous-probex`, `previous-dist/`, rollback images,
  `manifest.json`, new artifacts and verification. Remote rebuild artifacts
  `probex`/`dist/` match this release. The transfer archive was removed.
- Browser rollout pending: the Chrome extension was observed at 1.1.0, loading
  `../probex-webrtc-guidex-extension` correctly. The source is 1.1.2, but macOS
  locked before reload; neither the extension nor GuideX page was reloaded.
  Do not claim end-to-end acceptance until reload and a fresh user interaction.
  There were zero Runtime v4 results at server verification.

### Previous: 20260910T032218Z

- `20260910T032218Z`: deployed and verified on 2026-09-10 at 03:26 UTC.
- Image: `probex-hk-frontend:guidex-v4-20260910T032218Z`, also tagged `latest`.
- Main asset: `/assets/index-Ceowby04.js`; SHA-256:
  `c5f19f740dd91a17d7e7a2b1c92b1debe808bcf51afa829257f0e60084391b4c`.
- All served release files matched the uploaded artifacts. HTTPS SPA routes,
  health and authenticated read APIs passed; Chrome rendered after a hard reload.
- Backend ID/start time, ports, mounts and deployment configuration hashes stayed
  unchanged. No results were deleted. There were zero v4 results at verification;
  live per-turn acceptance still needs a fresh Runtime v4 sample.
