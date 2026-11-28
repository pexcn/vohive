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

在 QMI 设备的 SIM 卡设置中选择仅驻网或蜂窝数据，即可打开用户态 VoLTE。也可以通过 `PUT /api/cards/{iccid}/policy` 启用。请求体示例为
`{"volte_enabled":true,"airplane_enabled":false,"apply_device_id":"你的设备 ID"}`。
`volte_enabled` 随 ICCID 保存。VoLTE 和 VoWiFi 共用 IMS 会话，切换时先关闭旧会话。
`volte_enabled:false` 可关闭 VoLTE。

VoLTE 需要支持 QMAP 的 QMI 模组、能建立 IMS 专用 APN 的 SIM 和容器网络管理权限。首次切换至 QMAP 时，已有的普通数据连接可能重新拨号。IMS 在独立的 mux 中使用运营商下发的地址与 P-CSCF，普通数据仍使用独立的默认数据承载。`ims_access` 和 `bearer_ready` 可从设备运行态读取。IMS 完成注册后使用现有通话接口。

## LICENSE

本项目基于 [PolyForm Noncommercial License 1.0.0](LICENSE) 开源。
