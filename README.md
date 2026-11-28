# VoHive

[![License: PolyForm Noncommercial 1.0.0](https://img.shields.io/badge/License-PolyForm--Noncommercial--1.0.0-blue.svg)](https://polyformproject.org/licenses/noncommercial/1.0.0)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](go.mod)
[![Vue 3](https://img.shields.io/badge/Vue-3-42b883?logo=vue.js)](web/package.json)

> 面向高通 4G/LTE/5G 模组（Quectel EC20/EC25/EC21/EG25/EM20 等）的综合管理与代理服务平台。

VoHive 把模组热插拔管理、SOCKS5/HTTP 代理编排、短信收发、VoWiFi/IMS 通话、eSIM 全生命周期管理整合到一个服务里,并提供一套现代化的响应式 Web 管理后台。

## 核心特性

| 模块 | 说明 |
| --- | --- |
| 多模组并发管理 | USB 热插拔自动发现(ttyUSB 等)、多设备实时状态监控 |
| 轻量级代理引擎 | 内建 SOCKS5 / HTTP 代理内核,支持多实例并发;基于 `SO_BINDTODEVICE` 按设备网卡严格绑定出站流量 |
| 通信与短信中心 | 统一界面/API 处理 AT 短信收发、会话与联系人管理、USSD 交互,短信落库可查 |
| eSIM 管理 | 通过 AT 指令通道直接管理 eSIM 芯片,支持 Profile 下载、启用/停用、重命名、删除 |
| 全渠道通知 | 重要短信及系统告警可推送至 Telegram、Email、PushPlus、Bark、飞书(Lark/Feishu)、QQ 等 |
| 多架构构建 | 原生支持 amd64 / arm64 / arm7 跨平台编译,路由器到边缘节点均可部署 |

## 典型应用场景

- **私有 IP 代理池**:单主机挂载多张物理 SIM 卡或多张 eSIM,每张网卡对应独立的 SOCKS5/HTTP 实例,组建自己的移动网络代理。
- **统一接码/验证码中心**:Web 界面或 API 并行收发多卡短信,并通过 Webhook/Bot 实时推送到个人终端。
- **VoWiFi 零信号通信**:地下室、弱覆盖场景下,借助宽带网络隧道建立 IMS 连接,保证业务不掉线。

## 架构与技术栈

- **Backend**:Go 1.26+(Gin、GORM、Viper、euicc-go)
- **Frontend**:Vue 3 + Vite + TailwindCSS + Element Plus
- **Database**:SQLite(`vohive.db`)
- **CI/CD**:GitHub Actions 自动化多架构 Docker 镜像构建与发布

## 用户态 VoLTE

在 QMI 或 MBIM 设备的 SIM 卡设置中选择仅驻网或蜂窝数据，即可打开用户态 VoLTE。也可以通过 `PUT /api/cards/{iccid}/policy` 启用。请求体示例为
`{"volte_enabled":true,"airplane_enabled":false,"apply_device_id":"你的设备 ID"}`。
`volte_enabled` 随 ICCID 保存。VoLTE 和 VoWiFi 共用 IMS 会话，切换时先关闭旧会话。
`volte_enabled:false` 可关闭 VoLTE。

VoLTE 需要可用的 IMS 专用 APN 和容器网络管理权限。QMI 使用独立 QMAP mux，首次启用可能触发普通数据重新拨号。MBIM 使用独立 IMS 会话，需要模组支持至少两个 IP 会话，以及 Linux `cdc_mbim` 的 VLAN 会话接口。普通数据使用默认会话。设备运行态的 `ims_access` 和 `bearer_ready` 显示接入方式和承载状态。IMS 完成注册后，通话和短信使用现有接口。VoLTE 尚未就绪时，短信发送会返回冲突状态。

卡策略中的 APN 和 IPv4／IPv6 设置变更后，QMI 或 MBIM 会按新配置重拨普通数据。QMI 的 `/api/health` 和拨号响应包含 `data_plane_health`，DNS 不通会显示诊断状态，仍可使用已有 IP 连接。

## 运营商画像

内置画像可离线使用。将本地的 `selectors/` 和 `profiles/` 放到 `data/carrier-profiles/` 可启用 Apple Bundle v3 选择和展开。本地 JSON 覆盖仍具有最高优先级。需要云端画像时，在服务配置中加入下列设置。

```yaml
vowifi:
  carrier_profiles_url: https://raw.githubusercontent.com/iniwex5/ios-carrier-profiles/main
```

服务只请求归属 PLMN 对应的选择清单和匹配的画像，IMSI、ICCID、SPN、GID 在本机匹配。成功获取的文件保存在 `data/carrier-profiles-cache/`，云端不可用时可继续读缓存。显式配置的 ePDG 备用地址沿用该设备的 VoWiFi SOCKS5 DNS 和出口设置。

## SIP 短信桥接

启用后，软电话或 PBX 可以使用 UDP、Digest SHA-256 注册，并发送和接收纯文本 SIP MESSAGE。每个账号绑定一台模组。下面是配置示例，`advertise_host` 填写客户端能访问的 NAS 地址。

```yaml
sip_bridge:
  enabled: true
  listen_addr: 0.0.0.0:5060
  advertise_host: 192.168.1.10
  accounts:
    - username: modem1
      password: "<替换为该账号的密码>"
      device_id: modem1
      bearer: auto
```

不填写监听地址时只监听 `127.0.0.1:5060`。容器通过端口映射接入时需映射 UDP 端口。客户端用相同账号注册，将 MESSAGE 的 Request-URI 设置为 `sip:+88612345678@NAS地址`。`bearer` 支持 `auto`、`ims` 和 `cellular`，沿用 VoLTE 未就绪时的发送保护。

客户端收到 SIP 202 后，可用响应里的 `X-VoHive-SMS-ID` 查询 `/api/sip-bridge/sms/{message_id}`。202 表示已入队，`submitted` 表示发往运营商的提交已完成。IMS 消息可用返回的 `phone_message_id` 查询现有投递状态接口。模组收到的短信在 SIP 客户端离线时保留，注册后自动投递。管理员可通过 `/api/sip-bridge/status`、`/api/sip-bridge/sms/pending` 查看状态，用 `POST /api/sip-bridge/sms/{message_id}/retry` 重试发给 SIP 客户端的任务。发往手机号的结果不明时，任务标记为 `unknown`，可先核对投递结果。

## 代理出口和 DNS

在代理实例设置中选择指定模组或 NAS 默认网络。旧实例继续使用原绑定模组和系统解析。对 HTTP 与 SOCKS5 都可选择让 DNS 随出口解析，并填写 DNS 服务器 IP 或 IP 加端口。多个服务器按填写顺序尝试，用逗号分隔，例如 `1.1.1.1,9.9.9.9`。模组若提供 DNS，会优先用于未填写服务器的实例。其它情况下使用默认解析器地址 `1.1.1.1:53`，IPv6-only 网络可填写可用的 IPv6 DNS。

SOCKS5 支持 UDP ASSOCIATE，沿用实例出口与 DNS。UDP 转发可以逐实例关停。容器需要让客户端能访问 UDP 中继动态端口，Linux 部署可使用 host networking。VoWiFi 上游 SOCKS5 的 ePDG DNS 仍由卡的 VoWiFi 配置控制。

## LICENSE

本项目基于 [PolyForm Noncommercial License 1.0.0](LICENSE) 开源。
