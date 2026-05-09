# HAS Device Simulator — 第二阶段技术规格

本文是 `has-device-simulator` 第二阶段的实现规格，给 AI 直接照着实现即可。AI 不需要再去翻阅 SL100_Service 主仓库，本文档已经把所有需要的协议字段、数据结构、行为细节抄写过来。

---

## 0. 阅读顺序

1. §1 范围 — 明确这一轮要做什么、不要做什么。
2. §2 协议总览 — 复习已有 topic / envelope / 签名规则。
3. §3 数据模型 — 这是本阶段最重要的产物。新的 `shadow` 字段、`Users` 列表、`UpgradeStatus` 等都基于这里。
4. §4 ~ §6 — 函数、事件、属性的逐项契约。
5. §7 — 代码改动的落点（按现有目录布局给出）。
6. §8 — scenario 扩展。
7. §9 — 验收清单与手动联调脚本。

---

## 1. 范围

### 1.1 必做

- 把以下 5 个 func 从“能回包”升级成“协议正确 + 假数据状态变化”：
  - `AddPwd`
  - `UpdatePwd`
  - `DelPwd`
  - `UnBind`
  - `OtaUpgrade`
- 增加用户管理的 func（已有 `CreateUser` 雏形，本阶段补全为可联动 `Users` shadow 字段的版本）：
  - `CreateUser`（升级，不是新增）
  - `UpdateUser`
  - `DelUser`
- shadow 扩展：
  - `Users` 列表（含每个用户的 `pwd / face / palm / fp / nfc` 五种凭证子列表）
  - `UpgradeStatus`（`step` + `schedule`）
  - `Version`、`Battery / CurrentBattery`、`LockState / LockStatus`、`Online`、`Ringing` 等已有键的统一管理
- 事件扩展：
  - `UpgradeStart`（OTA 启动 / 进度变更）
  - `UpgradeResult`（OTA 结果）
  - `Reboot`（重启上报）
- CLI / scenario：
  - 新增 scenario：`bind-online-add-pwd`、`bind-online-ota`
  - `event` 命令支持上述新事件名

### 1.2 不做

- 不接真实音视频、蓝牙、P2P。
- 不做多设备并发调度（仍然是单设备常驻）。
- 不引入持久化数据库；shadow 仅内存态即可，重启后状态丢失是可以接受的。
- 不修改 `bind / login` 协议（已经能跑通）。
- 不实现真实的 OTA 文件下载，下载/安装阶段全部用伪状态机模拟。

### 1.3 约束沿用 AGENTS.md

- 单设备常驻优先。
- 仿真精度：协议正确 + 假数据，不追求真实业务规则。
- 凭证模式继续保留 `credentials.mode = fixed` 的开发态方案。

---

## 2. 协议总览（已有，列出供对照）

### 2.1 MQTT topic 模板

所有 topic 以 `/thing/<model>/<uuid>/...` 为前缀。已有：

| 用途 | 模板 |
| --- | --- |
| 属性下行 | `/thing/<model>/<uuid>/attr/set` |
| 属性下行回执 | `/thing/<model>/<uuid>/attr/set/resp` |
| 属性主动上报 | `/thing/<model>/<uuid>/attr/post` |
| 属性查询 | `/thing/<model>/<uuid>/attr/get` |
| 属性查询响应 | `/thing/<model>/<uuid>/attr/get/resp` |
| func 下行 | `/thing/<model>/<uuid>/func/<FuncName>` |
| func 回执 | `/thing/<model>/<uuid>/func/<FuncName>/resp` |
| 事件上报 | `/thing/<model>/<uuid>/event/<EventName>` |

本阶段不新增 topic 模板，新增的 func / event 都用上面这套。

### 2.2 通用 envelope

设备订阅到的请求和设备主动上报的负载，都是 JSON 对象，最小字段：

```json
{
  "msg_id": "string",
  "time": 1715123456789,
  "data": { ... }
}
```

设备回 func 的响应固定形态：

```json
{
  "msg_id": "原 msg_id",
  "time": 1715123456789,
  "result": 1,
  "data": { ... },
  "err_code": 0,
  "msg": "ok"
}
```

约定：

- `result = 1` 表示成功，`result = 0` 表示失败。
- `err_code` 仅失败时填，可参考 §6.4。
- `time` 使用毫秒时间戳。已有代码部分位置用了秒，本阶段统一改成毫秒。

### 2.3 设备签名

已经实现于 `internal/protocol/signature.go`，本阶段不动。新加的 HTTP 请求（如果有）必须复用 `protocol.BuildDeviceSignature`。

---

## 3. 数据模型

### 3.1 shadow 总览

`internal/shadow.State` 当前只是两个 `map[string]any`。本阶段保留这个内存结构，但把以下属性的“类型”和“含义”规范下来。所有这些属性必须能：

1. 被 `attr/set` 下行修改并回执。
2. 在变化时通过 `attr/post` 主动上报。
3. 在 `attr/get/resp` 收到的对应键时，覆盖到本地。

### 3.2 标准属性键

| Key | 类型 | 含义 | 默认值 |
| --- | --- | --- | --- |
| `Online` | int | 是否在线，0/1 | `1` 上线时上报 |
| `Version` | string | 固件版本 | 取自 `device.version` |
| `LockState` | string | 锁状态：`locked` / `unlocked` | `locked` |
| `LockStatus` | int | 锁状态枚举：`0=unlocked, 1=locked` | `1` |
| `Battery` | int | 当前电量百分比 | `80` |
| `LowBattery` | int | 低电报警阈值 | `20` |
| `Charging` | int | 是否充电 0/1 | `0` |
| `Ringing` | bool | 是否在响铃 | `false` |
| `DoorLastAction` | string | `open_success` / `open_failed` 等 | 空 |
| `Volume` | int | 音量百分比 0..100 | `65` |
| `SleepState` | int | 睡眠状态 | `0` |
| `LightStatus` | int | 灯光状态 | `1` |
| `UpgradeStatus` | object `{step:int, schedule:int}` | OTA 进度 | `{step:0, schedule:0}` |
| `WIFI` | object `{ssid, pwd}` | WIFI 信息 | `{ssid:"", pwd:""}` |
| `RSSI` | int | WIFI 强度 | `-100` |
| `Users` | array<User> | 用户列表 | `[]` |

`LockState` 和 `LockStatus` 二选一也行，但要保持一致：本阶段两者都维护，互为镜像（`locked <-> 1, unlocked <-> 0`）。

### 3.3 `Users` 结构

```go
type AttributeUser struct {
    Id   string             `json:"id"`   // 用户 id
    Name string             `json:"name"` // 用户名
    Role int                `json:"role"` // 0=管理员, 1=普通用户, 2=临时用户（仅约定，不强校验）
    Pwd  []AttributeUserPwd `json:"pwd"`  // 密码槽
    Face []AttributeUserPwd `json:"face"` // 人脸
    Palm []AttributeUserPwd `json:"palm"` // 掌静脉
    Fp   []AttributeUserPwd `json:"fp"`   // 指纹
    Nfc  []AttributeUserPwd `json:"nfc"`  // NFC
}

type AttributeUserPwd struct {
    Id   string `json:"id"`   // 凭证 id
    Data string `json:"data"` // 数据本体（密码/特征/卡号），加密或明文均可
    Enc  string `json:"enc"`  // 加密算法标识，未加密填 ""
    Exp  int64  `json:"exp"`  // 过期时间戳，0 表示永不过期
}
```

凭证类型枚举（仅在 func payload 里使用，shadow 内部用上面五个数组区分）：

```text
1 = Key（钥匙）
2 = Pwd（密码）
3 = Fingerprint（指纹）
4 = Face（人脸）
5 = Cloud（云端）
6 = Palm（掌静脉）
7 = NFC
```

实现要求：

- 维护一个 `internal/shadow/users.go` 的小工具集合：`AddUser / UpdateUser / DelUser / AddPwd / UpdatePwd / DelPwd`，全部通过 `State.SetReported({"Users": [...]})` 落到 shadow。
- 写完之后不要忘记 `attr/post` 把新的 `Users` 上报一次。

### 3.4 `UpgradeStatus`

```go
type UpgradeStatus struct {
    Step     int `json:"step"`     // 0=空闲, 1=下载中, 2=校验中, 3=安装中, 4=收尾, 5=成功, 6=失败
    Schedule int `json:"schedule"` // 0..100，与 step 相关的进度
}
```

调度规则见 §4.5。

---

## 4. func 实现规格

下面所有函数都通过订阅 `/thing/<model>/<uuid>/func/+` 收到，处理完毕后必须回 `/thing/<model>/<uuid>/func/<FuncName>/resp`。

### 4.1 `Lock` / `Unlock`（已实现，规范化）

请求：

```json
{ "msg_id": "...", "time": 1715000000000, "data": {} }
```

行为：

- `Lock`：`SetReported({"LockState":"locked", "LockStatus":1})`，再 `attr/post` 这两个键。
- `Unlock`：`SetReported({"LockState":"unlocked", "LockStatus":0})`，再 `attr/post`。

回执：

```json
{ "msg_id": "...", "time": 1715000000000, "result": 1, "status": "success" }
```

### 4.2 `CreateUser` / `UpdateUser` / `DelUser`

#### 4.2.1 `CreateUser`

请求 `data`：

```json
{ "name": "Alice", "role": 1 }
```

行为：

- 生成新的 `id`（用 `time.Now().Unix()` 字符串即可，和参考实现保持一致）。
- 在 shadow `Users` 里 append 一个 `AttributeUser`，五个凭证子列表都初始化为空数组。
- `attr/post` 上报新的 `Users`。

回执 `data`：

```json
{ "id": "1715000000" }
```

#### 4.2.2 `UpdateUser`

请求 `data`：

```json
{ "user_id": "1715000000", "name": "Alice2", "role": 1 }
```

行为：

- 找到对应用户，更新 `name` 和 `role`。
- 找不到：`result=0`，`err_code=40401`，`msg="user not found"`。
- 成功后 `attr/post` 上报 `Users`。

回执 `data`：可省略或返回 `{ "id": "<user_id>" }`。

#### 4.2.3 `DelUser`

请求 `data`（参考实现复用了 UpdateUser 的结构，但只用 `user_id`）：

```json
{ "user_id": "1715000000" }
```

行为：

- 从 `Users` 里删除该用户。
- 找不到时回 `result=0, err_code=40401`。
- 成功后 `attr/post` 上报 `Users`。
- 同时上报事件 `DelUser`（见 §5.5）。

### 4.3 `AddPwd`

请求 `data`：

```json
{
  "user_id": "1715000000",
  "type": 2,
  "data": "123456",
  "enc": "",
  "exp": 0
}
```

行为：

- 找到 `user_id` 对应的用户。
- 按 `type` 把新凭证 append 到对应子列表（`type=2 -> Pwd`，`type=3 -> Fp`，`type=4 -> Face`，`type=6 -> Palm`，`type=7 -> Nfc`，`type=1/5` 当前不存槽位，但要正常回成功）。
- 凭证 id 规则：`fmt.Sprintf("%s_%d", user_id, time.Now().Unix())`。
- `attr/post` 上报新的 `Users`。
- 同时上报事件 `AddPwd`（见 §5.5）。

回执 `data`：

```json
{ "id": "1715000000_1715000123" }
```

错误：

- 用户不存在：`result=0, err_code=40401, msg="user not found"`。
- `type` 不在 1..7：`result=0, err_code=40002, msg="invalid type"`。

### 4.4 `UpdatePwd` / `DelPwd`

#### 4.4.1 `UpdatePwd`

请求 `data`：

```json
{
  "user_id": "1715000000",
  "type": 2,
  "pwd_id": "1715000000_1715000123",
  "data": "654321",
  "enc": "",
  "exp": 0
}
```

行为：

- 在对应用户的对应凭证子列表里找到 `pwd_id`，覆盖其 `data / enc / exp`。
- `attr/post` 上报 `Users`。

回执：成功 `result=1`。

#### 4.4.2 `DelPwd`

请求 `data`：

```json
{ "user_id": "1715000000", "type": 2, "pwd_id": "1715000000_1715000123" }
```

行为：

- 在对应子列表里删除 `pwd_id`。
- `attr/post` 上报 `Users`。
- 同时上报事件 `DelPwd`（见 §5.5）。

错误：找不到一律 `result=0, err_code=40401`。

### 4.5 `OtaUpgrade`

请求 `data`：

```json
{
  "version": "1.1.0",
  "md5": "abcdef...",
  "expire_time": 1715999999,
  "uri": "https://example.com/firmware.bin"
}
```

行为（关键是“假状态机”）：

1. 立刻回 func resp，`result=1, status="accepted", data={"accepted": true}`。
2. 异步启动一个 goroutine：
   - `step` 从 0 开始，每 `500ms` 推进一步：1 -> 2 -> 3 -> 4 -> 5。
   - 每次 step 推进都：
     - `SetReported({"UpgradeStatus": {"step": step, "schedule": step*20}})`
     - `attr/post UpgradeStatus`
     - 上报事件 `UpgradeStart`，`data={"step": step, "schedule": step*20}`。
   - `step=5` 时表示成功；上报事件 `UpgradeResult`，`data={"result":1, "msg":"OTA升级成功", "version": "<新版本>", "time": <ms>}`。
   - 失败模拟：可以先不做随机失败。如果做，规则是“在 step=1..4 任意一步上以 1% 概率切到失败”，此时上报 `UpgradeResult`，`data={"result":0, "msg": OtaFailureMessage[step], "time": <ms>}`，并停止推进。
3. 成功后：
   - `SetReported({"Version": payload.Version, "UpgradeStatus": {"step":0, "schedule":0}})`
   - `attr/post Version`、`attr/post UpgradeStatus`。

`OtaFailureMessage`（用作可选失败时的文案）：

```text
0: 更新初始化失败
1: 固件包下载失败
2: 安装初始化失败
3: 安装失败
4: 安装失败
```

注意：`OtaUpgrade` 的“立刻回 resp + 异步推进”这点必须做对，否则 App 会判超时。

### 4.6 `UnBind`

请求 `data`：可以是空对象。

行为：

1. 立刻回 func resp，`result=1, status="unbinding", data={"accepted": true}`。
2. 同步上报事件 `UnBind`（payload 见 §5.5）。
3. 异步：
   - 等 1 秒。
   - `mqttsession.Disconnect(250)`。
   - 把 shadow 重置成默认值（参见 §3.2 默认值）。
   - 在日志里打印“已解绑，等待外部重新触发 bind”。
4. **不要 `os.Exit`**。参考实现是 CLI 工具会退出进程，但本仓库的目标是常驻联调工具，解绑后只清状态、断 MQTT，让外部测试可以再调一次 `bind` 命令。

### 4.7 `Reboot`（最小骨架，但要可见）

请求 `data`：可以是空对象。

行为：

1. 立刻回 func resp，`result=1, status="rebooting"`。
2. 异步等 500ms：
   - 上报事件 `Reboot`。
   - 重新发一次 `Online=1` 的 `attr/post`。
   - `attr/post` 当前 `Version`。

不需要真的断开 MQTT。

### 4.8 未支持函数

如果收到不在上面清单的函数名：

- `result=0, err_code=40001, status="unsupported", data={"reason":"function not implemented in simulator"}`。
- 当前 `behavior.engine.go` 已经是这个语义，保持。

---

## 5. event 实现规格

事件全部通过 `/thing/<model>/<uuid>/event/<EventName>` 主动上报，envelope 是：

```json
{ "msg_id": "...", "time": <ms>, "data": { ... } }
```

下面只描述 `data` 的内容。所有事件都要在 `internal/behavior/engine.go` 的 `TriggerEvent` 路由 + `StaticEventFactory.Build` 增补对应分支，并在 `cmd/device-simulator` 的 `event` 命令上能用。

### 5.1 `Ring` / `RingStop`（已有，规范化）

`Ring.data`:

```json
{ "source":"doorbell", "face_id":"sim-face-001", "ringing":true, "battery":80, "scenario":"manual" }
```

`RingStop.data`:

```json
{ "reason":"timeout", "ringing":false, "scenario":"manual" }
```

副作用：`Ring -> SetReported({"Ringing":true})`，`RingStop -> SetReported({"Ringing":false})`，并 `attr/post Ringing`。

### 5.2 `OpenDoor`（已有，规范化）

`data`:

```json
{ "method":"fingerprint", "result":"success", "lock":"locked", "scenario":"manual" }
```

副作用：`SetReported({"DoorLastAction":"open_success"})` 并 `attr/post`。

### 5.3 `LowBattery`（已有，规范化）

`data`:

```json
{ "battery":20, "threshold":20, "result":"warning", "scenario":"manual" }
```

副作用：`SetReported({"Battery":20})` 并 `attr/post`。

### 5.4 `UpgradeStart` / `UpgradeResult`

由 `OtaUpgrade` 函数内部的状态机自动触发（见 §4.5）。也允许通过 `event UpgradeStart` 命令手动触发一次（用一个固定 step=1, schedule=20 的 payload），便于联调时单独验证事件。

`UpgradeStart.data`:

```json
{ "step": 2, "schedule": 40, "version": "1.1.0" }
```

`UpgradeResult.data`:

```json
{ "result": 1, "msg": "OTA升级成功", "version": "1.1.0", "time": 1715000000000 }
```

### 5.5 用户/凭证联动事件

- `AddPwd.data`:

  ```json
  { "user_id": "1715000000", "type": 2, "pwd_id": "1715000000_1715000123" }
  ```

- `DelPwd.data`:

  ```json
  { "user_id": "1715000000", "type": 2, "pwd_id": "1715000000_1715000123" }
  ```

- `DelUser.data`:

  ```json
  { "user_id": "1715000000" }
  ```

- `SetUsers.data`（每次 `Users` 变化时可选地额外发，便于 App 端整体刷新；本阶段先做成 `DelUser / AddPwd / DelPwd` 后内部再发一次 `SetUsers`，payload 是当前完整 `Users` 数组）：

  ```json
  { "users": [ { "id":"...", "name":"...", "role":1, "pwd":[], "face":[], "palm":[], "fp":[], "nfc":[] } ] }
  ```

### 5.6 `Reboot`

`data`:

```json
{ "reason":"manual", "version":"1.0.0", "time": 1715000000000 }
```

### 5.7 其他参考事件名（不需要本阶段做，列出供命名一致）

`Tamper / Answer / Pir / Radar / Stay / OpenDoorFailedMax / WaitingLockTimeout / CpuUsage / MemoryUsage`。如果下一阶段要做，沿用同样的 envelope 即可。

---

## 6. 通用规则

### 6.1 时间戳

- 所有 envelope 的 `time` 字段统一使用 **毫秒** (`time.Now().UnixMilli()`)。
- 已有代码个别位置用了秒（参考实现里 `MQTTSendEvent` 用的 `time.Now().Unix()`），本阶段统一成毫秒。

### 6.2 msg_id

- 设备主动上报：`fmt.Sprintf("sim-%d", time.Now().UnixNano())`，沿用现有逻辑。
- func resp / attr/set resp：必须回原 `msg_id`，不要自己生成。

### 6.3 attr/post 触发原则

任何引起 shadow 变化的入口（attr/set、func、event、scenario 步骤），改完 shadow 之后必须发一次 `attr/post`，只带本次变化的键，不要把整个 shadow dump 出去。

### 6.4 错误码

| 场景 | err_code |
| --- | --- |
| 函数未实现 | 40001 |
| 参数非法 | 40002 |
| 资源不存在（user / pwd） | 40401 |
| 状态冲突（如 OTA 进行中又收到 OTA） | 40901 |
| 内部错误 | 50000 |

只在 `result=0` 时填。

### 6.5 并发

- shadow 已经有 `sync.RWMutex`，新增的 `Users` 读写一律走 `SetReported / Snapshot`。
- OTA 状态机用一个独立 goroutine。同一时刻只允许一个 OTA 在跑：发现已经在跑就回 `result=0, err_code=40901, msg="upgrade in progress"`。

---

## 7. 代码改动落点

按现有目录结构（已 verified）增量修改：

```
internal/protocol/
    mqtt.go               // 不动
    signature.go          // 不动
internal/shadow/
    state.go              // 保留
    users.go              // 新增：用户/凭证操作辅助函数（见 §3.3）
    upgrade.go            // 新增：UpgradeStatus 结构 + 状态机推进辅助
internal/behavior/
    engine.go             // 扩展：HandleFunc 路由新增 AddPwd/UpdatePwd/DelPwd/UnBind/OtaUpgrade/UpdateUser/DelUser/Reboot；TriggerEvent 路由新增 UpgradeStart/UpgradeResult/Reboot/AddPwd/DelPwd/DelUser/SetUsers
    users.go              // 新增：func 子处理器
    ota.go                // 新增：OtaUpgrade 状态机
    unbind.go             // 新增：UnBind 流程
internal/scenario/
    scenario.go           // 新增 bind-online-add-pwd / bind-online-ota / bind-online-unbind
internal/app/
    app.go                // RunScenario 支持新 step 类型：func（通过 behavior 直接调 HandleFunc 模拟一次下发）
cmd/device-simulator/
    main.go               // event/scenario 命令的 --name 帮助文本更新
configs/
    sl100-dev.yaml        // 不动
docs/
    PHASE2_SPEC.md        // 本文件
    IMPLEMENTATION_NOTES.md // 完工后更新当前实现边界
```

`behavior.engine.go` 当前已经是 switch 路由的形态，新增分支即可。建议把每个复杂分支抽到独立文件（`users.go / ota.go / unbind.go`）以保持单文件可读。

`Publisher` 接口已经有 `PublishAttrSetResp / PublishFuncResp / PublishEvent`，足够使用。如果 OTA 的 `attr/post` 需要新方法，可以直接复用 `mqttsession.Client.PublishAttrPost`；为此可以把 `Publisher` 接口加一个 `PublishAttrPost(ctx, map[string]any) error` 方法，并在 `mqttsession.Client` 上对应实现已存在。

### 7.1 scenario step 类型扩展

`scenario.Step` 当前只有 `Action` 和 `Target`。本阶段扩展：

```go
type Step struct {
    Action string         // bind/connect/login/online/event/func/sleep
    Target string         // event 名 / func 名
    Data   map[string]any // func 时的请求 data；event 时可覆盖默认 data
    Wait   time.Duration  // sleep 时的等待时间
}
```

`internal/app/app.go::RunScenario` 的 switch 增加：

```go
case "func":
    req := protocol.FunctionRequest{
        Name:  step.Target,
        MsgID: fmt.Sprintf("sim-%d", time.Now().UnixNano()),
        Time:  time.Now().UnixMilli(),
        Data:  marshalOrEmpty(step.Data),
    }
    if err := a.behavior.HandleFunc(ctx, req); err != nil { return err }
case "sleep":
    select {
    case <-time.After(step.Wait):
    case <-ctx.Done():
        return ctx.Err()
    }
```

### 7.2 新增 scenario

```go
{
    Name:        "bind-online-add-pwd",
    Description: "bind, online, create user, then AddPwd",
    Steps: []Step{
        {Action: "bind"}, {Action: "connect"}, {Action: "login"}, {Action: "online"},
        {Action: "func", Target: "CreateUser", Data: map[string]any{"name":"Alice","role":1}},
        {Action: "sleep", Wait: 200 * time.Millisecond},
        // user_id 在 scenario 里不知道，可以用一个特殊字段 "$last_user_id"，由 RunScenario 在 CreateUser 之后从 shadow 里读出来填入。
        {Action: "func", Target: "AddPwd", Data: map[string]any{"user_id":"$last_user_id","type":2,"data":"123456","enc":"","exp":0}},
    },
},
{
    Name: "bind-online-ota",
    Steps: []Step{
        {Action: "bind"}, {Action: "connect"}, {Action: "login"}, {Action: "online"},
        {Action: "func", Target: "OtaUpgrade", Data: map[string]any{"version":"1.1.0","md5":"x","expire_time":1715999999,"uri":"http://x"}},
        {Action: "sleep", Wait: 4 * time.Second}, // 等 OTA 状态机跑完
    },
},
{
    Name: "bind-online-unbind",
    Steps: []Step{
        {Action: "bind"}, {Action: "connect"}, {Action: "login"}, {Action: "online"},
        {Action: "func", Target: "UnBind"},
    },
},
```

`$last_user_id` 这种占位符让 `RunScenario` 在执行 `func` 步骤前，从 `shadow.Snapshot().Reported["Users"]` 取出最后一条用户的 `id` 替换进去。占位符匹配只做最简实现：直接遍历 `step.Data` 的字符串值，等于 `"$last_user_id"` 时替换。

---

## 8. 配置变更

`configs/sl100-dev.yaml` 不动。`internal/config/config.go` 不动。

可选：`behavior` 段可以加一个 `ota_step_interval_ms`（默认 `500`），方便调慢/调快 OTA 演示。如果加，要在 `BehaviorConfig` 上加字段，并在 OTA 状态机里读取；不加也可以，硬编码 500ms 也行。

---

## 9. 验收

完工后请同步更新：

- `docs/IMPLEMENTATION_NOTES.md`：把“已实现”分组扩展上 `AddPwd / UpdatePwd / DelPwd / UpdateUser / DelUser / OtaUpgrade / UnBind / Reboot` 和新事件、新 scenario。
- `AGENTS.md`：把“当前状态”里这几条从“最小骨架”移到“具备”，并把“下一步优先级”里的对应项划掉。

### 9.1 单元测试

新增以下测试，全部用纯内存 publisher mock：

- `internal/shadow/users_test.go`
  - `AddUser` / `UpdateUser` / `DelUser` 基础读写。
  - `AddPwd` 五种类型分别落到正确的子列表。
  - `UpdatePwd` / `DelPwd` 命中 / 未命中分支。
- `internal/behavior/engine_test.go`（已有，扩展用例）
  - `CreateUser -> AddPwd -> DelPwd` 链路：每一步检查 `Users` 状态、检查 publisher 收到正确的 `func/resp`、`attr/post`、对应 `event`。
  - `OtaUpgrade` 链路：注入一个 `ota_step_interval_ms=10` 的快版本，断言 publisher 收到 `step=1..5` 全部 `attr/post` + `UpgradeStart` 事件 + 一次 `UpgradeResult` 事件 + 最后一次 `attr/post Version`。
  - `UnBind` 链路：断言先有 `func/resp`，再有 `event/UnBind`，shadow 关键键被清空。

### 9.2 手动联调脚本

文档末尾给出一组可复制粘贴的命令，按顺序执行：

```bash
# 终端 A：让模拟器常驻
go run ./cmd/device-simulator run --config ./configs/sl100-dev.yaml

# 终端 B：用 mosquitto_pub 模拟后端下发
TOPIC=/thing/SL100/SIM-LOCK-0001/func

mosquitto_pub -t "$TOPIC/CreateUser" -m '{"msg_id":"m1","time":1715000000000,"data":{"name":"Alice","role":1}}'
mosquitto_pub -t "$TOPIC/AddPwd"     -m '{"msg_id":"m2","time":1715000000000,"data":{"user_id":"<上一步返回的 id>","type":2,"data":"123456","enc":"","exp":0}}'
mosquitto_pub -t "$TOPIC/Lock"       -m '{"msg_id":"m3","time":1715000000000,"data":{}}'
mosquitto_pub -t "$TOPIC/OtaUpgrade" -m '{"msg_id":"m4","time":1715000000000,"data":{"version":"1.1.0","md5":"x","expire_time":1715999999,"uri":"http://x"}}'
mosquitto_pub -t "$TOPIC/UnBind"     -m '{"msg_id":"m5","time":1715000000000,"data":{}}'
```

预期：

- 每条 `func` 都能在终端 A 看到收到、回 `func/resp`。
- `CreateUser / AddPwd` 之后能看到 `attr/post` 带最新 `Users`。
- `Lock` 之后能看到 `attr/post {"LockState":"locked","LockStatus":1}`。
- `OtaUpgrade` 之后能看到约 5 次 `attr/post UpgradeStatus`，5 次 `event/UpgradeStart`，1 次 `event/UpgradeResult`，最后一次 `attr/post Version`。
- `UnBind` 之后能看到 `event/UnBind`，紧接着 MQTT 断开，模拟器仍在运行（不退出进程）。

### 9.3 不允许的实现偏差

- 不要把 `Users` 拆成多个 shadow 键（必须是单键的数组）。
- 不要在 OTA 里同步阻塞回 resp（必须立即回 resp + 异步推进）。
- 不要在 `UnBind` 里调 `os.Exit`。
- 不要把 `time` 字段在不同 publisher 里混用秒/毫秒，统一毫秒。

---

## 10. 完成定义（DoD）

- [ ] §4 列出的所有 func 都按规格回包并触发对应 shadow 变化与事件。
- [ ] §5 列出的所有事件都能通过 `event` 命令或函数副作用触发。
- [ ] `internal/shadow.Users / UpgradeStatus` 数据结构与方法到位。
- [ ] 三个新 scenario 跑通且行为符合 §9.1 / §9.2 描述。
- [ ] 新增 / 扩展的单元测试全部通过：`go test ./...`。
- [ ] `docs/IMPLEMENTATION_NOTES.md` 与 `AGENTS.md` 已同步更新。
