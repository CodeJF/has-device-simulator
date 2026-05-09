# has-device-simulator

独立的 Go CLI 工具，模拟门锁设备接入 MQTT，与 HAS SmartLock 后端 / App 完成 `bind / login / func / event` 联调。

定位：**协议正确 + 假数据状态机**。不接真实音视频 / 蓝牙 / P2P，不模拟真实业务规则。

> 完整功能矩阵、协议契约、shadow 字段、错误码、联调命令请看 [`docs/FEATURES.md`](docs/FEATURES.md)。本 README 只负责快速上手。

## 当前能力一览

- HTTP 接入：`POST /v1/device/bind`、`POST /v1/device/login`，含 HMAC-SHA256 设备签名
- MQTT 会话：连接、订阅 `attr/set` / `attr/get/resp` / `func/+`，自动重连
- 11 个 func：`Lock` / `Unlock` / `CreateUser` / `UpdateUser` / `DelUser` / `AddPwd` / `UpdatePwd` / `DelPwd` / `OtaUpgrade` / `UnBind` / `Reboot`
- 12 类 event：`Ring` / `RingStop` / `OpenDoor` / `LowBattery` / `UpgradeStart` / `UpgradeResult` / `Reboot` / `AddPwd` / `DelPwd` / `DelUser` / `SetUsers` / `UnBind`
- shadow 内存态：`Online / Version / LockState / LockStatus / Battery / LowBattery / Charging / Ringing / DoorLastAction / Volume / SleepState / LightStatus / UpgradeStatus / WIFI / RSSI / Users`
- OTA 假状态机：异步 5 步推进 + 互斥保护
- 6 个内置 scenario：`bind-online` / `bind-online-ring` / `bind-online-open-door` / `bind-online-add-pwd` / `bind-online-ota` / `bind-online-unbind`
- `UnBind` 不退出进程，重置 shadow 后等待外部重新 `bind`

## 目录结构

```text
cmd/device-simulator/   # CLI 入口
internal/app/           # 编排层（Run / Bind / RunScenario / TriggerEvent）
internal/config/        # YAML + viper + SIM_* 环境变量
internal/deviceprofile/ # 设备身份
internal/provisioning/  # bind / login HTTP client（含签名）
internal/protocol/      # MQTT envelope、func/event 协议、签名实现
internal/mqttsession/   # paho mqtt 封装（订阅 + 发布）
internal/shadow/        # 内存态影子（Desired / Reported）+ Users / UpgradeStatus
internal/behavior/      # func 路由 + 事件工厂 + OTA / UnBind / Reboot 流程
internal/scenario/      # 内置 scenario 定义
configs/                # YAML 配置（真实文件已 gitignore，仓库内只有 *.example.yaml）
docs/                   # FEATURES.md / IMPLEMENTATION_NOTES.md / PHASE2_SPEC.md
```

## 配置初始化

`configs/sl100-dev.yaml` 含开发态凭证，已加入 `.gitignore`，不进仓库。第一次 clone 后基于模板生成本地配置：

```bash
cp configs/sl100-dev.example.yaml configs/sl100-dev.yaml
# 编辑 sl100-dev.yaml，填入真实值：
#   backend.api_base_url     后端地址
#   mqtt.broker              MQTT broker
#   device.model_secret      设备签名 secret
#   credentials.fixed_password  MQTT 登录密码（fixed 模式）
```

也支持环境变量覆盖（前缀 `SIM_`，`.` → `_`），例：

```bash
SIM_MQTT_BROKER=tcp://10.0.0.5:1883 go run ./cmd/device-simulator
```

## 常用命令

```bash
# 默认子命令是 run，IDE 可直接传 --config
go run ./cmd/device-simulator --config ./configs/sl100-dev.yaml

# 仅跑 bind+login，验证签名 / HTTP 头 / 凭证
go run ./cmd/device-simulator bind --config ./configs/sl100-dev.yaml

# 全程驻留：bind → login → MQTT → attr/post Online=1，等 SIGINT
go run ./cmd/device-simulator run --config ./configs/sl100-dev.yaml

# 执行内置 scenario
go run ./cmd/device-simulator scenario --config ./configs/sl100-dev.yaml --name bind-online
go run ./cmd/device-simulator scenario --config ./configs/sl100-dev.yaml --name bind-online-add-pwd
go run ./cmd/device-simulator scenario --config ./configs/sl100-dev.yaml --name bind-online-ota
go run ./cmd/device-simulator scenario --config ./configs/sl100-dev.yaml --name bind-online-unbind

# 上线后立刻发一条事件
go run ./cmd/device-simulator event --config ./configs/sl100-dev.yaml --name Ring
go run ./cmd/device-simulator event --config ./configs/sl100-dev.yaml --name LowBattery
```

## 测试

```bash
go test ./...
```

覆盖：`engine_test.go` / `state_test.go` / `users_test.go` / `mqtt_test.go` / `signature_test.go` / `client_test.go` / `config_test.go` / `app_test.go` / `main_test.go`。

## 当前约束

- 凭证模式当前只支持 `fixed`，MQTT 密码取自 `credentials.fixed_password`；后端 `bind / login` 若返回真实 `mqtt_username / mqtt_password` 会优先使用，否则回退到 fixed
- 单设备常驻，不做并发调度
- shadow 全部内存态，进程重启后 `Users / UpgradeStatus / OTA 后的 Version` 会丢
- OTA 不真下载文件、不做 MD5 校验、不做失败分支
- 周期 `heartbeat_seconds / attr_post_seconds` 字段已读取但未驱动定时上报

## 文档分工

- `README.md` — 快速上手（本文件）
- [`docs/FEATURES.md`](docs/FEATURES.md) — 完整功能矩阵 + 协议契约 + 联调速查
- [`docs/IMPLEMENTATION_NOTES.md`](docs/IMPLEMENTATION_NOTES.md) — 当前实现边界
- [`docs/PHASE2_SPEC.md`](docs/PHASE2_SPEC.md) — 第二阶段技术规格（实现依据）
- [`AGENTS.md`](AGENTS.md) — 项目记忆 / 当前状态 / 下一步优先级
