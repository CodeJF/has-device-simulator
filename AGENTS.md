# HAS Device Simulator Memory

本仓库是独立的 Go 设备模拟器项目。

目标是模拟真实门锁设备接入 MQTT，并和 HAS SmartLock 后端、App 完成 `bind / login / func / event` 联调。

## 当前状态

- 当前已有 CLI 命令：
  - `run`
  - `bind`
  - `scenario`
  - `event`
- 当前主链路已经具备：
  - `bind -> login -> MQTT connect -> attr/post Online=1`
- 当前最小 func 行为已经具备：
  - `AddPwd`
  - `UpdatePwd`
  - `DelPwd`
  - `Lock`
  - `Unlock`
  - `CreateUser`
  - `UpdateUser`
  - `DelUser`
  - `Reboot`
  - `UnBind`
  - `OtaUpgrade`
- 当前最小事件行为已经具备：
  - `Ring`
  - `RingStop`
  - `OpenDoor`
  - `LowBattery`
  - `UpgradeStart`
  - `UpgradeResult`
  - `Reboot`
  - `AddPwd`
  - `DelPwd`
  - `DelUser`
  - `SetUsers`
  - `UnBind`
- 当前固定场景已经具备：
  - `bind-online`
  - `bind-online-ring`
  - `bind-online-open-door`
  - `bind-online-add-pwd`
  - `bind-online-ota`
  - `bind-online-unbind`
- 当前 shadow 已覆盖第二阶段联调所需关键字段：
  - `Users`
  - `UpgradeStatus`
  - `Version`
  - `Battery / LockState / LockStatus / Online / Ringing`
- 当前凭证策略仍是开发态方案：
  - `credentials.mode = fixed`
  - `credentials.fixed_password = debug-fixed-password`

## 明确未完成

- 还没有批量设备能力，当前默认是单设备常驻联调工具。
- 还没有复杂 scenario runner，当前仍是固定顺序场景为主。
- 还没有持久化状态，重启后 `Users / UpgradeStatus` 等内存态会丢失。
- 仿真仍以“协议正确 + 假数据”为准，未进入真实设备业务规则。

## 下一步优先级

1. 补设备控制增强：
   - 继续补齐剩余低优先级 func / event
2. 强化假数据状态精度，例如：
   - 更真实的用户/凭证业务约束
   - OTA 失败分支与进度细化
3. 评估是否进入更复杂的 scenario runner。
4. 视联调需求再决定是否补持久化或多设备能力。

## 当前默认约束

- 单设备常驻优先，不为多设备并发提前设计调度层。
- 仿真精度以“协议正确 + 假数据”为准，不追求真实设备内部业务规则。
- 当前不接真实音视频、蓝牙、P2P。
- 本仓库独立演进，不回写主后端仓库。

## 文档分工

- `README.md` 负责说明如何运行。
- `docs/IMPLEMENTATION_NOTES.md` 负责说明当前实现边界。
- `AGENTS.md` 只负责记录项目记忆、当前状态和下一步优先级。

后续每次阶段推进后，优先更新 `docs/IMPLEMENTATION_NOTES.md`，再同步更新本文件中的“当前状态”和“下一步优先级”段落。
