# has-device-simulator

`has-device-simulator` 是一个独立的 Go CLI 工具，用来模拟门锁设备接入 MQTT，并和 HAS SmartLock 后端完成 `bind / login / MQTT / func / event` 联调。

## 第一版目标

- 跑通 `POST /v1/device/bind`
- 跑通 `POST /v1/device/login`
- 建立 MQTT 连接
- 订阅 `/thing/<model>/<uuid>/func/+`
- 上报一次 `attr/post Online=1`
- 支持 `Lock`、`Unlock`、`CreateUser` 的最小 `func/resp`
- 支持 `Ring`、`RingStop`、`OpenDoor`、`LowBattery` 的最小事件上报

## 目录结构

```text
cmd/device-simulator/
internal/app/
internal/config/
internal/deviceprofile/
internal/provisioning/
internal/protocol/
internal/mqttsession/
internal/shadow/
internal/behavior/
internal/scenario/
configs/
docs/
```

## 常用命令

```bash
go run ./cmd/device-simulator run --config ./configs/sl100-dev.yaml
go run ./cmd/device-simulator --config ./configs/sl100-dev.yaml
go run ./cmd/device-simulator bind --config ./configs/sl100-dev.yaml
go run ./cmd/device-simulator scenario --config ./configs/sl100-dev.yaml --name bind-online
go run ./cmd/device-simulator scenario --config ./configs/sl100-dev.yaml --name bind-online-ring
go run ./cmd/device-simulator event --config ./configs/sl100-dev.yaml --name Ring
```

其中 `run` 是默认命令，所以 IDE 里直接启动二进制时，也可以只传：

```bash
--config ./configs/sl100-dev.yaml
```

## 当前约束

- 当前默认凭证模式是 `fixed`，MQTT 密码来自 `credentials.fixed_password`
- 这是开发态临时方案，便于先打通联调链路
- 后续如果后端提供开发态凭证接口，只需要替换 `internal/provisioning` 中 `ResolveMQTTCredentials` 的实现

## 配置初始化

`configs/sl100-dev.yaml` 已加入 `.gitignore`，不会进入仓库。第一次拉代码后请基于模板生成本地配置：

```bash
cp configs/sl100-dev.example.yaml configs/sl100-dev.yaml
# 然后把 model_secret / fixed_password / api_base_url / broker 改成你自己的值
```

## 下一步建议

1. 把 `configs/sl100-dev.yaml` 填成真实后端地址和真实 `model_secret`
2. 先执行 `bind` 验证签名和设备 HTTP 头是否匹配
3. 再执行 `run` 验证 MQTT 连接与 `attr/post Online=1`
4. 再执行 `event` 或 `scenario` 验证 `Ring / OpenDoor / LowBattery` 上报
5. 最后在 App 或 `/ws` 侧验证 `Lock / Unlock / CreateUser` 下发和 `func/resp` 回执
