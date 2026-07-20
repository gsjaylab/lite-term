<div align="center">

# 轻终端

**FN LiteTerm — 轻量、简单、即开即用的 fnOS 终端**

[![GitHub Release](https://img.shields.io/github/v/release/gsjaylab/lite-term?style=flat-square)](https://github.com/gsjaylab/liteterm/releases/latest)
[![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![TypeScript](https://img.shields.io/badge/TypeScript-7-blue?style=flat-square&logo=typescript&logoColor=white)](https://www.typescriptlang.org/)

</div>

---

## 项目简介

轻终端（FN LiteTerm）是一款运行在 fnOS 上的轻量终端应用。安装后可直接从 fnOS 桌面打开，通过登录弹窗连接本机 SSH 服务，验证成功后进入对应用户的 Shell，无需额外开放网络端口。

## 核心功能

- **完整终端体验**：基于 xterm.js，支持 ANSI 颜色、常用终端操作与窗口尺寸自适应
- **原生账号登录**：通过登录弹窗测试并连接本机 SSH 服务，可按当前 fnOS 用户记住端口、用户名和密码
- **实时双向通信**：通过 WebSocket 在浏览器与后端 SSH Session 之间传输终端输入和输出
- **会话自动回收**：关闭页面后自动结束当前终端会话，避免遗留后台进程
- **并发安全限制**：每个 fnOS 用户同一时间仅允许一个终端会话，单台设备最多并发 8 个会话
- **应用网关接入**：通过 fnOS 应用网关访问，不额外监听或暴露网络端口

## 使用条件

| 项目 | 要求 |
|:-----|:-----|
| 设备架构 | x86_64、ARM64（AArch64） |
| SSH 服务 | 已开启；登录弹窗中的端口默认使用 `22` |
| 用户权限 | 登录用户已获得 SSH 权限 |

## 技术栈

| 层级 | 技术 |
|:-----|:-----|
| 前端 | TypeScript + Vite + xterm.js |
| 后端 | Go 1.24+ |
| 实时通信 | WebSocket |
| 终端能力 | Go SSH Client + 本机 OpenSSH Server |
| 测试 | Vitest + Playwright + Go Test |
| 打包工具 | fnpack |

## 后续规划

- [ ] **优化交互体验**：继续完善连接状态、错误分类与重试流程
- [x] **适配 ARM 平台**：支持为 x86_64 和 ARM64 fnOS 设备分别构建原生安装包
- [ ] **终端个性化**：提供主题、字体大小、光标样式和快捷键等可配置选项
- [x] **连接配置管理**：按当前 fnOS 用户在应用数据目录保存端口、用户名和密码；文件权限限制为仅应用账号可读写
- [ ] **上架飞牛应用商店**：如果官方允许


## 本地打包

安装 fnOS 官方打包工具 `fnpack` 并确保其位于 `PATH` 中，然后执行：

```bash
./scripts/local-build-fpk.sh 0.0.1 x86

# ARM64 fnOS
./scripts/local-build-fpk.sh 0.0.1 arm
```

生成的安装包分别位于 `dist/liteterm-0.0.1-x86.fpk` 和 `dist/liteterm-0.0.1-arm.fpk`。

## 目录结构

```text
cmd/liteterm/       程序入口
internal/         Go 后端实现
web/              浏览器终端前端
packaging/fnos/   fnOS 应用配置和图标
scripts/          构建及打包脚本
```

## 开源协议

本项目基于 [MIT License](LICENSE) 开源。

版权所有 © 2026 GSJayLab
