# HAS Device Simulator — 功能说明

本文档基于当前仓库实际代码（非规格预想），梳理 `has-device-simulator` 在“模拟门锁设备”这件事上**到底做了什么**、**做到什么程度**。读完应能回答：哪些设备能力被仿真、协议契约长什么样、哪些是真实的、哪些是“假数据但协议正确”。

> 仿真精度定位：**协议正确 + 假数据状态机**。不接真实音视频/蓝牙/P2P，不模拟真实业务规则。

---

## 1. 总体能力地图

模拟器以单设备常驻进程为主，启动后大致流程：

```
[CLI] → bind (HTTP) → login (HTTP) → MQTT 连接 → 订阅 attr/set, attr/get/resp, func/+
                                              ↓
                                         attr/post Online=1
                                              ↓
                  [func 下行] ← MQTT broker ← [event 上行 / attr/post 上行]
```

按层划分：

| 层 | 实现位置 | 说明 |
| --- | --- | --- |
| 配置 | `internal/config` | YAML + viper，支持 `SIM_*` 环境变量覆盖 |
| 设备身份 | `internal/deviceprofile` | `model / uuid / uid / appid / mac / zone / version / secret / client_id` |
| 设备签名 | `internal/protocol/signature.go` | `HMAC-SHA256 → hex → base64`，规范化参数串 |
| HTTP 接入 | `internal/provisioning` | `POST /v1/device/bind`、`POST /v1/device/login`，凭证回退到 `fixed` 模式 |
| MQTT 会话 | `internal/mqttsession` | paho mqtt，订阅 + 发布封装 |
| 协议数据 | `internal/protocol/mqtt.go` | `Envelope / FunctionRequest / FunctionResponse` 等 |
| 设备影子 | `internal/shadow` | 内存态 `Desired / Reported`，含 `Users / UpgradeStatus` 等 |
| 行为引擎 | `internal/behavior` | func 路由、事件工厂、OTA 状态机、UnBind/Reboot 流程 |
| 场景脚本 | `internal/scenario` | 6 个内置 `Definition`，`bind → connect → login → online → func/event/sleep` |
| 应用编排 | `internal/app` | 把上述层组装起来，提供 `Run / Bind / RunScenario / TriggerEvent` |
| CLI | `cmd/device-simulator` | 4 个子命令：`run / bind / scenario / event` |

---

## 2. CLI 子命令

入口：`cmd/device-simulator/main.go`。

```
device-simulator [run|bind|scenario|event] --config <path> [--name <scenario-or-event>]
```

| 命令 | 行为 |
| --- | --- |
| `run`（默认） | bind → login → MQTT 连接 → 订阅 → `attr/post Online=1`，常驻直到 `SIGINT/SIGTERM` |
| `bind` | 仅跑 `bind + login`，验证签名/HTTP 头/凭证返回 |
| `scenario --name <name>` | 跑一个内置脚本（见 §7） |
| `event --name <name>` | 上线后立刻发一条事件（见 §6） |

未带子命令时（如 IDE 直跑二进制），自动当成 `run`。

---

## 3. 接入与认证（HTTP 层）

### 3.1 设备签名 `BuildDeviceSignature`

签名串构造：

```
upper(method) + "&" + headerString + "&" + canonicalParams
```

- `headerString` = `model + request_id + timestamp + uuid`，若 `uid` 非空则在 `timestamp` 与 `uuid` 之间插入 `uid`（与后端 login 接口口径一致）
- `canonicalParams`：键升序排序、`k=v` 拼接、`&` 连接；`GET / DELETE` 时 value 走 `encodeURIComponent`，其它方法不编码
- 数组按逗号拼接、`bool / int / float` 标量化、`map / 嵌套对象`回退为 `[object Object]`（与 JS 端实现保持兼容）
- 最终签名：`base64( hex( HMAC_SHA256(secret, base) ) )`

### 3.2 `bind / login`

`internal/provisioning/client.go`：

- `POST /v1/device/bind` body：`{uid, mac, zone, version}`，header 带 `model / uuid / appid / timestamp / request_id / sign`
- `POST /v1/device/login` body：`{zone, version}`，额外带 `uid` header，签名也包含 `uid`
- 同时识别 `code = 0` 与 `code = 1000` 两种成功码（兼容历史与现行后端）
- 优先从响应 `data` 里抽取 `mqtt_username / mqtt_password`，抽不到就 fallback 到 **`fixed` 凭证模式**：`username = uuid`，`password = credentials.fixed_password`，`client_id = profile.client_id`

凭证模式当前硬编码到 `fixed` / `""` 两种，其它模式直接 `unsupported`。

---

## 4. MQTT 会话与 topic

### 4.1 已订阅 topic

设备侧统一前缀 `/thing/<model>/<uuid>/...`，启动时订阅 3 条：

| topic | 含义 |
| --- | --- |
| `attr/set` | 后端下行属性写 |
| `attr/get/resp` | 设备主动 query 后的响应（当前无主动 get，但留了 handler） |
| `func/+` | 所有功能下行（`Lock / Unlock / CreateUser / ...`），通过路径段路由到具体 func |

### 4.2 设备主动发布

| topic | 用途 | 接口 |
| --- | --- | --- |
| `attr/post` | 上报变化属性 | `PublishAttrPost` |
| `attr/set/resp` | 回 `attr/set` | `PublishAttrSetResp` |
| `func/<Name>/resp` | 回 func 调用 | `PublishFuncResp` |
| `event/<Name>` | 主动事件 | `PublishEvent` |

### 4.3 envelope 约定

- 所有 envelope `time` 字段统一 **毫秒**（`time.Now().UnixMilli()`）
- 设备主动上报：`msg_id = sim-<UnixNano>`
- func 回执 / attr/set 回执：原样回 `msg_id`
- func 回执 schema：`{msg_id, time, result, err_code?, msg, status, data}`，`result=1` 成功，`result=0` 失败时填 `err_code`
- 错误码：`40001 unsupported / 40002 invalid / 40401 not_found / 40901 conflict / 50000 internal`

---

## 5. 设备影子（Shadow）

`internal/shadow/state.go` 用一对 `map[string]any` + `sync.RWMutex` 维护 `Desired / Reported`，对外暴露 `SetDesired / SetReported / Snapshot / Reset`，并对嵌套结构做深拷贝。

### 5.1 默认 Reported 字段

`DefaultReportedState(version)` 一次性铺出第二阶段联调需要的全部 key：

| Key | 类型 | 默认值 | 含义 |
| --- | --- | --- | --- |
| `Online` | int | `0`（联机后置 1） | 在线状态 |
| `Version` | string | 取自 `device.version` | 固件版本 |
| `LockState` | string | `"locked"` | 锁状态文本 |
| `LockStatus` | int | `1` | 锁状态枚举（0=unlocked, 1=locked） |
| `Battery` | int | `80` | 电量 % |
| `LowBattery` | int | `20` | 低电阈值 |
| `Charging` | int | `0` | 是否充电 |
| `Ringing` | bool | `false` | 是否响铃 |
| `DoorLastAction` | string | `""` | 最近一次开门结果 |
| `Volume` | int | `65` | 音量 |
| `SleepState` | int | `0` | 睡眠 |
| `LightStatus` | int | `1` | 灯光 |
| `UpgradeStatus` | `{step,schedule}` | `{0,0}` | OTA 进度 |
| `WIFI` | `{ssid,pwd}` | `{"",""}` | WIFI 信息 |
| `RSSI` | int | `-100` | WIFI 强度 |
| `Users` | `[]AttributeUser` | `[]` | 用户列表 |

### 5.2 `LockState ↔ LockStatus` 镜像

`normalizeAttrPatch` 在写入前会自动同步两者：

- `LockState=locked` ⇄ `LockStatus=1`
- `LockState=unlocked` ⇄ `LockStatus=0`

任何方向上的下行/上报都会保证两个字段一致。

### 5.3 `Users` 结构

```go
AttributeUser{ Id, Name, Role, Pwd, Face, Palm, Fp, Nfc }
AttributeUserPwd{ Id, Data, Enc, Exp }
```

- `Pwd` 子列表对应 `type=2`、`Fp`→3、`Face`→4、`Palm`→6、`Nfc`→7
- `type=1 (Key) / type=5 (Cloud)` 当前**不落槽位**（落到任何子列表都会丢失），但 `AddPwd` 仍返回成功，避免影响协议联调
- 每次 `Users` 变化都通过 `attr/post {"Users": [...]}` 整列上报；部分入口还会额外发 `event/SetUsers` 便于 App 整体刷新

### 5.4 `UpgradeStatus`

```go
UpgradeStatus{ Step, Schedule }
NewUpgradeStatus(step) // schedule = step * 20
```

`step` 值约定（与规格一致）：`0=空闲 1=下载 2=校验 3=安装 4=收尾 5=成功 6=失败`，当前 OTA 状态机只跑 1→5（无失败分支）。

---

## 6. 设备功能仿真（func 下行）

入口 `behavior.DefaultEngine.HandleFunc`，按 `req.Name` 路由。**未识别的 func** 一律回：

```json
{"result":0,"err_code":40001,"status":"unsupported",
 "data":{"reason":"function not implemented in simulator"}}
```

下表是已实现的全部 func：

| 函数名 | 入参 `data` | 主要副作用 | 回执 |
| --- | --- | --- | --- |
| `Lock` | `{}` | `LockState=locked, LockStatus=1` → `attr/post` | `result=1, status=success` |
| `Unlock` | `{}` | `LockState=unlocked, LockStatus=0` → `attr/post` | `result=1, status=success` |
| `CreateUser` | `{name, role}` | append 一个 `AttributeUser`（id=`<UnixSec>`），五个凭证子列表初始化为空 → `attr/post Users` | `data={id}` |
| `UpdateUser` | `{user_id, name, role}` | 找到则改 `name/role` 并 `attr/post Users` | 找不到 `40401` |
| `DelUser` | `{user_id}` | 删用户 → `attr/post Users` → `event/DelUser` → `event/SetUsers` | 找不到 `40401` |
| `AddPwd` | `{user_id, type, data, enc, exp}` | 按 `type` 落到对应子列表（1/5 不存槽），凭证 id=`<user_id>_<UnixSec>` → `attr/post Users` → `event/AddPwd` → `event/SetUsers` | `data={id}`；用户找不到 `40401`，type∉[1..7] `40002` |
| `UpdatePwd` | `{user_id, type, pwd_id, data, enc, exp}` | 覆盖 `data/enc/exp` → `attr/post Users` | 找不到 `40401`，type 非法 `40002` |
| `DelPwd` | `{user_id, type, pwd_id}` | 删凭证 → `attr/post Users` → `event/DelPwd` → `event/SetUsers` | 找不到 `40401` |
| `OtaUpgrade` | `{version, md5, expire_time, uri}` | **立刻 accept** + 异步状态机：每 500ms 推进 `step 1..5`，每步 `attr/post UpgradeStatus` + `event/UpgradeStart`；step=5 后 `event/UpgradeResult` + `attr/post {Version, UpgradeStatus={0,0}}`。同一时间只允许一个 OTA，二次进入返回 `40901 conflict` | 立即 `result=1, status=accepted, data={accepted:true}` |
| `UnBind` | `{}` | 立即 `event/UnBind`；异步 1s 后 `Disconnect(250)` + `Shadow.Reset(version)`；**不退出进程** | 立即 `result=1, status=unbinding, data={accepted:true}` |
| `Reboot` | `{}` | 异步 500ms 后 `event/Reboot` + `attr/post Online=1` + `attr/post Version`（不真断 MQTT） | 立即 `result=1, status=rebooting` |

> 入参解析使用 `decodeStruct`，未传字段按零值处理。`payload` 解析失败回 `40002 invalid`。

---

## 7. 事件上行（event）

事件统一通过 `event/<Name>` 主动发布，envelope 标准化为 `{msg_id, time(ms), data}`。事件来源有两种：

1. **func 副作用**：例如 `OtaUpgrade` 内部跑出来的 `UpgradeStart / UpgradeResult`、`AddPwd` 跑出来的 `event/AddPwd + event/SetUsers`。
2. **`event` 子命令 / `scenario` 中 `Action=event` 步骤**：调用 `behavior.TriggerEventWithData`，由 `StaticEventFactory.Build` 构造默认 payload，外部可覆盖。

`StaticEventFactory` 已实现的事件名与默认 payload：

| 事件 | 默认 `data` | 副作用（attr/post） |
| --- | --- | --- |
| `Ring` | `{source:"doorbell", face_id:"sim-face-001", ringing:true, battery:<shadow.Battery>, scenario:"manual"}` | `Ringing=true` |
| `RingStop` | `{reason:"timeout", ringing:false, scenario:"manual"}` | `Ringing=false` |
| `OpenDoor` | `{method:"fingerprint", result:"success", lock:<shadow.LockState>, scenario:"manual"}` | `DoorLastAction="open_success"` |
| `LowBattery` | `{battery:20, threshold:20, result:"warning", scenario:"manual"}` | `Battery=<payload.battery>` |
| `UpgradeStart` | `{step:1, schedule:20, version:<shadow.Version>}` | `UpgradeStatus={step,schedule}` |
| `UpgradeResult` | `{result:1, msg:"OTA升级成功", version, time(ms)}` | — |
| `Reboot` | `{reason:"manual", version, time(ms)}` | — |
| `AddPwd` | `{user_id, type, pwd_id}`（从最近一个用户读出来，找不到回 `manual-*` 占位） | — |
| `DelPwd` | 同上 | — |
| `DelUser` | `{user_id:<最近用户 id 或 "manual-user">}` | — |
| `SetUsers` | `{users: <当前 Users 列表>}` | — |
| `UnBind` | `{reason:"manual"}` | — |

未识别的事件名报错（`unsupported event`）。

---

## 8. 内置场景（scenario）

`internal/scenario/scenario.go` 定义了 6 个 `Definition`，通过 `Action` 串起来执行：

| 场景名 | 步骤序列 | 用途 |
| --- | --- | --- |
| `bind-online` | bind → connect → login → online | 最小联调链路 |
| `bind-online-ring` | bind/connect/login/online → `event Ring` | 验证响铃事件 |
| `bind-online-open-door` | … → `event OpenDoor` | 验证开门事件 |
| `bind-online-add-pwd` | … → `func CreateUser` → sleep 200ms → `func AddPwd(user_id=$last_user_id, type=2, ...)` | 验证用户/凭证联动 |
| `bind-online-ota` | … → `func OtaUpgrade` → sleep 4s | 让 OTA 状态机跑完一轮 |
| `bind-online-unbind` | … → `func UnBind` | 验证解绑流程（不退出进程） |

实现细节：

- `Action` 支持 `bind / connect / login`（在 `setupSession` 一并完成，作为占位）、`online`（`attr/post Online=1`）、`event`、`func`、`sleep`
- 占位符 **`$last_user_id`**：在跑 `func` / `event` 步骤前，遍历 `step.Data` 字符串值，等于 `$last_user_id` 时替换为 `Users` 数组里最后一个用户的 `Id`（实现见 `app.resolveScenarioData` 与 `shadow.LastUserID`）
- `func` 步骤通过 `behavior.HandleFunc` 直接喂入一条**伪造的下行**，等价于 broker 推过来一条 `func/<Name>`

---

## 9. 配置文件

`configs/sl100-dev.yaml` 字段说明：

```yaml
backend:
  api_base_url: "http://127.0.0.1:8080"   # 必填
  timeout_seconds: 10
mqtt:
  broker: "tcp://127.0.0.1:1883"          # 必填
  keepalive_seconds: 30
  clean_session: true
  client_id: ""                           # 留空时用 "<model>_<uuid>"
device:
  model: "SL100"                          # 必填
  uuid: "SIM-LOCK-0001"                   # 必填
  uid: "u_demo_dev"
  appid: "smartlock"                      # 必填
  mac: "00:11:22:33:44:55"
  zone: "Asia/Shanghai"
  version: "1.0.0"
  model_secret: "<32位>"                  # 必填，签名 secret
behavior:
  heartbeat_seconds: 60                   # 已读取，未启用周期心跳
  attr_post_seconds: 0                    # 已读取，未启用周期 attr/post
credentials:
  mode: "fixed"                           # 当前唯一可用模式
  fixed_password: "<MQTT 密码>"
```

支持 `SIM_*` 环境变量覆盖（如 `SIM_MQTT_BROKER`）。

---

## 10. 已实现 vs 未实现

### 10.1 已具备

- HTTP `bind / login` + 设备签名（含 `uid` / 非 `uid` 两种 header 排列）
- MQTT 连接、固定 topic 订阅、自动重连、签名鉴权（用 `uuid + fixed_password`）
- shadow 的全部第二阶段标准键 + 锁状态镜像 + `Users` / `UpgradeStatus` 结构化解析
- 11 个 func：`Lock / Unlock / CreateUser / UpdateUser / DelUser / AddPwd / UpdatePwd / DelPwd / OtaUpgrade / UnBind / Reboot`
- 12 类 event：`Ring / RingStop / OpenDoor / LowBattery / UpgradeStart / UpgradeResult / Reboot / AddPwd / DelPwd / DelUser / SetUsers / UnBind`
- OTA 假状态机（异步 5 步推进、上报 `UpgradeStatus / UpgradeStart / UpgradeResult / Version`、互斥保护）
- `UnBind` 不退出进程，重置 shadow 等待外部再 `bind`
- 6 个内置 scenario，`func` 步骤支持 `$last_user_id` 占位符
- 单元测试覆盖：`engine_test.go / state_test.go / users_test.go / mqtt_test.go / signature_test.go / client_test.go / config_test.go / app_test.go / main_test.go`

### 10.2 明确未做

- **多设备并发**：单设备常驻，没有调度层
- **持久化**：shadow 全部内存态，进程重启后 `Users / UpgradeStatus / Version`（OTA 后变更）会丢
- **真实业务规则**：用户/凭证不做唯一性、过期、容量、权限校验；OTA 不真下载文件、不做 MD5 校验、不做失败分支
- **真实音视频 / 蓝牙 / P2P**：均不接入
- **凭证模式**：除 `fixed` 外都不支持；`bind / login` 接口若返回真实 MQTT 凭证会优先使用，否则回退 fixed
- **周期任务**：`heartbeat_seconds / attr_post_seconds` 字段已读取，但未驱动定时心跳或定时 attr/post
- **复杂 scenario runner**：当前是顺序步骤数组，没有分支、循环、条件、并发等高级语法
- **未支持的 func / event**：规格里列出但本阶段不做的 `Tamper / Answer / Pir / Radar / Stay / OpenDoorFailedMax / WaitingLockTimeout / CpuUsage / MemoryUsage` 等；命中未支持函数会按 `40001 unsupported` 回执

---

## 11. 联调速查（结合 §6 / §7）

> mosquitto broker 已开启用户名/密码鉴权，模拟器登录用的也是同一对凭证。所以下发命令时必须显式带 `-u / -P`，否则会在 broker 上直接被拒。
>
> 凭证来源：`username = device.uuid`（见 `configs/sl100-dev.yaml`），`password = credentials.fixed_password`（见 `provisioning.ResolveMQTTCredentials`）。客户端 ID 不能与模拟器的 `<model>_<uuid>` 冲突，所以 publisher 自带一个独立 ID。

```bash
# 终端 A：常驻
go run ./cmd/device-simulator run --config ./configs/sl100-dev.yaml

# 终端 B：模拟后端下发（broker / 凭证按 configs/sl100-dev.yaml）
HOST=127.0.0.1
PORT=1883
USER=SIM-LOCK-0001
PASS=OXYZ0123456789ab
TOPIC=/thing/SL100/SIM-LOCK-0001/func

PUB="mosquitto_pub -h $HOST -p $PORT -u $USER -P $PASS -i sim-pub-$$ -q 1"

$PUB -t "$TOPIC/CreateUser" -m '{"msg_id":"m1","time":1715000000000,"data":{"name":"Alice","role":1}}'
$PUB -t "$TOPIC/AddPwd"     -m '{"msg_id":"m2","time":1715000000000,"data":{"user_id":"<上一步 id>","type":2,"data":"123456","enc":"","exp":0}}'
$PUB -t "$TOPIC/Lock"       -m '{"msg_id":"m3","time":1715000000000,"data":{}}'
$PUB -t "$TOPIC/OtaUpgrade" -m '{"msg_id":"m4","time":1715000000000,"data":{"version":"1.1.0","md5":"x","expire_time":1715999999,"uri":"http://x"}}'
$PUB -t "$TOPIC/UnBind"     -m '{"msg_id":"m5","time":1715000000000,"data":{}}'
```

如果想顺手订阅设备上行（`func/<Name>/resp`、`event/<Name>`、`attr/post`），用同一对凭证即可：

```bash
mosquitto_sub -h $HOST -p $PORT -u $USER -P $PASS -i sim-sub-$$ -q 1 \
  -t '/thing/SL100/SIM-LOCK-0001/#' -v
```

常见报错速查：

- `Connection Refused: not authorised` → `-u / -P` 没带，或与 `configs/sl100-dev.yaml` 里 `device.uuid` / `credentials.fixed_password` 不一致
- `Connection Refused: identifier rejected` 或上来就被踢 → publisher 的 `-i` 客户端 ID 撞到了模拟器自身的 `SL100_SIM-LOCK-0001`，换一个就行

预期：

- `CreateUser / AddPwd` 回执后能收到 `attr/post {Users:[...]}` 与 `event/AddPwd / event/SetUsers`
- `Lock` 回执后收到 `attr/post {LockState:"locked", LockStatus:1}`
- `OtaUpgrade` 立即收到 `accepted` 回执，随后约 5 次 `attr/post UpgradeStatus` + 5 次 `event/UpgradeStart` + 1 次 `event/UpgradeResult` + 最终 `attr/post {Version, UpgradeStatus}`
- `UnBind` 立即收到 `unbinding` 回执 + `event/UnBind`，约 1s 后 MQTT 断开，进程不退出
