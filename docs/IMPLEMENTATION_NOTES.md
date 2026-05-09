# Implementation Notes

第二阶段实现边界：

- 已实现：
  - 配置加载
  - 设备签名
  - `bind / login` HTTP client
  - MQTT 连接与固定 topic 订阅
  - `Lock / Unlock` 状态联动与 `attr/post`
  - `CreateUser / UpdateUser / DelUser`
  - `AddPwd / UpdatePwd / DelPwd`
  - `Reboot / UnBind / OtaUpgrade`
  - `Ring / RingStop / OpenDoor / LowBattery`
  - `UpgradeStart / UpgradeResult / Reboot / AddPwd / DelPwd / DelUser / SetUsers / UnBind`
  - shadow 标准键统一维护：
    `Online / Version / LockState / LockStatus / Battery / LowBattery / Charging / Ringing / DoorLastAction / Volume / SleepState / LightStatus / UpgradeStatus / WIFI / RSSI / Users`
  - `Users`、密码槽位、OTA 进度的内存态假数据联动
  - `run / bind / scenario / event` CLI
  - `bind-online`、`bind-online-ring`、`bind-online-open-door`
  - `bind-online-add-pwd`、`bind-online-ota`、`bind-online-unbind`
- 预留扩展：
  - 更真实的设备内部业务规则和状态机细节
  - 更完整的 shadow 同步边角场景
  - 批量设备和复杂 scenario runner
